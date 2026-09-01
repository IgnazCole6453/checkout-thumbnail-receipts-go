package main

import (
	"bytes"
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"time"
)

const imageProcessPath = "/v1/image/process"

type ImageProcessor interface {
	Resize(context.Context, ResizeRequest) (ProcessedImage, error)
}

type ResizeRequest struct {
	Image          []byte
	Filename       string
	Width          int
	Height         int
	Fit            string
	Format         string
	Store          bool
	IdempotencyKey string
}

type ProcessedImage struct {
	ID  string `json:"id"`
	URL string `json:"url"`
}

type InfraiError struct {
	Code       string
	Message    string
	HTTPStatus int
}

func (e *InfraiError) Error() string {
	return fmt.Sprintf("image processing rejected: %s: %s", e.Code, e.Message)
}

type InfraiImageClient struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
	sleep      func(context.Context, time.Duration) error
}

func NewInfraiImageClient(baseURL, apiKey string, httpClient *http.Client) *InfraiImageClient {
	return &InfraiImageClient{
		baseURL:    baseURL,
		apiKey:     apiKey,
		httpClient: httpClient,
		sleep: func(ctx context.Context, delay time.Duration) error {
			timer := time.NewTimer(delay)
			defer timer.Stop()
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-timer.C:
				return nil
			}
		},
	}
}

type imageEnvelope struct {
	OK       bool            `json:"ok"`
	Data     json.RawMessage `json:"data"`
	Error    *envelopeError  `json:"error"`
	Metadata json.RawMessage `json:"metadata"`
}

type envelopeError struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

func (c *InfraiImageClient) Resize(ctx context.Context, input ResizeRequest) (ProcessedImage, error) {
	for attempt := 0; attempt < 4; attempt++ {
		body, contentType, err := resizeBody(input)
		if err != nil {
			return ProcessedImage{}, err
		}

		req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+imageProcessPath, body)
		if err != nil {
			return ProcessedImage{}, err
		}
		req.Header.Set("Authorization", "Bearer "+c.apiKey)
		req.Header.Set("Content-Type", contentType)
		req.Header.Set("Idempotency-Key", input.IdempotencyKey)

		response, err := c.httpClient.Do(req)
		if err != nil {
			return ProcessedImage{}, fmt.Errorf("send image request: %w", err)
		}
		payload, readErr := io.ReadAll(response.Body)
		response.Body.Close()
		if readErr != nil {
			return ProcessedImage{}, fmt.Errorf("read image response: %w", readErr)
		}

		var envelope imageEnvelope
		if err := json.Unmarshal(payload, &envelope); err != nil {
			return ProcessedImage{}, fmt.Errorf("decode image envelope: %w", err)
		}
		if !envelope.OK {
			if response.StatusCode == http.StatusTooManyRequests && attempt < 3 {
				if err := c.sleep(ctx, retryDelay(response.Header.Get("Retry-After"), attempt)); err != nil {
					return ProcessedImage{}, err
				}
				continue
			}
			apiErr := &InfraiError{HTTPStatus: response.StatusCode}
			if envelope.Error != nil {
				apiErr.Code = envelope.Error.Code
				apiErr.Message = envelope.Error.Message
			}
			return ProcessedImage{}, apiErr
		}
		if response.StatusCode >= 500 {
			return ProcessedImage{}, fmt.Errorf("image transport status %d", response.StatusCode)
		}

		var image ProcessedImage
		if err := json.Unmarshal(envelope.Data, &image); err != nil {
			return ProcessedImage{}, fmt.Errorf("decode image data: %w", err)
		}
		return image, nil
	}
	return ProcessedImage{}, fmt.Errorf("image request retry budget exhausted")
}

func resizeBody(input ResizeRequest) (*bytes.Buffer, string, error) {
	payload := struct {
		Image  map[string]string `json:"image"`
		Ops    []map[string]any  `json:"ops"`
		Format string            `json:"format"`
		Store  bool              `json:"store"`
	}{
		Image: map[string]string{"base64": base64.StdEncoding.EncodeToString(input.Image)},
		Ops: []map[string]any{{
			"op": "resize",
			"params": map[string]any{
				"width":  input.Width,
				"height": input.Height,
				"fit":    input.Fit,
			},
		}},
		Format: input.Format,
		Store:  input.Store,
	}
	body := new(bytes.Buffer)
	if err := json.NewEncoder(body).Encode(payload); err != nil {
		return nil, "", err
	}
	return body, "application/json", nil
}

func retryDelay(header string, attempt int) time.Duration {
	if seconds, err := strconv.Atoi(header); err == nil && seconds >= 0 {
		return time.Duration(seconds) * time.Second
	}
	return time.Duration(1<<attempt) * 200 * time.Millisecond
}
