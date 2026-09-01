package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"mime/multipart"
	"net/http"
	"strings"
)

const maxCheckoutBody = 24 << 20

type CheckoutService struct {
	images ImageProcessor
}

type CheckoutReceipt struct {
	OrderID        string      `json:"order_id"`
	CustomerEmail  string      `json:"customer_email"`
	ProductSKU     string      `json:"product_sku"`
	Status         string      `json:"status"`
	Thumbnails     []Thumbnail `json:"thumbnails"`
	CustomerUpdate string      `json:"customer_update"`
}

type Thumbnail struct {
	ImageID string `json:"image_id"`
	URL     string `json:"url"`
	Width   int    `json:"width"`
	Height  int    `json:"height"`
}

func NewCheckoutService(images ImageProcessor) *CheckoutService {
	return &CheckoutService{images: images}
}

func (s *CheckoutService) Handler() http.Handler {
	mux := http.NewServeMux()
	mux.HandleFunc("POST /orders/checkout", s.checkout)
	return mux
}

func (s *CheckoutService) checkout(w http.ResponseWriter, r *http.Request) {
	r.Body = http.MaxBytesReader(w, r.Body, maxCheckoutBody)
	if err := r.ParseMultipartForm(maxCheckoutBody); err != nil {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid multipart checkout"})
		return
	}

	orderID := strings.TrimSpace(r.FormValue("order_id"))
	email := strings.TrimSpace(r.FormValue("customer_email"))
	sku := strings.TrimSpace(r.FormValue("product_sku"))
	files := r.MultipartForm.File["images"]
	if orderID == "" || email == "" || sku == "" || len(files) == 0 {
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "order_id, customer_email, product_sku, and images are required"})
		return
	}

	receipt, err := s.buildReceipt(r.Context(), orderID, email, sku, files)
	if err != nil {
		status := http.StatusBadGateway
		var apiErr *InfraiError
		if errors.As(err, &apiErr) && apiErr.HTTPStatus >= 400 && apiErr.HTTPStatus < 500 {
			status = apiErr.HTTPStatus
		}
		writeJSON(w, status, map[string]string{
			"order_id": orderID,
			"status":   "thumbnail_review",
			"error":    err.Error(),
		})
		return
	}
	writeJSON(w, http.StatusCreated, receipt)
}

func (s *CheckoutService) buildReceipt(ctx context.Context, orderID, email, sku string, files []*multipart.FileHeader) (CheckoutReceipt, error) {
	thumbnails := make([]Thumbnail, 0, len(files))
	for index, header := range files {
		file, err := header.Open()
		if err != nil {
			return CheckoutReceipt{}, fmt.Errorf("open product image: %w", err)
		}
		content, readErr := io.ReadAll(file)
		file.Close()
		if readErr != nil {
			return CheckoutReceipt{}, fmt.Errorf("read product image: %w", readErr)
		}
		processed, err := s.images.Resize(ctx, ResizeRequest{
			Image: content, Filename: header.Filename, Width: 640, Height: 640,
			Fit: "cover", Format: "webp", Store: true,
			IdempotencyKey: fmt.Sprintf("checkout-%s-%d-640x640-webp", orderID, index),
		})
		if err != nil {
			return CheckoutReceipt{}, err
		}
		thumbnails = append(thumbnails, Thumbnail{ImageID: processed.ID, URL: processed.URL, Width: 640, Height: 640})
	}
	return decideReceipt(orderID, email, sku, thumbnails), nil
}

func decideReceipt(orderID, email, sku string, thumbnails []Thumbnail) CheckoutReceipt {
	status := "thumbnail_review"
	update := "Product images are being prepared."
	if len(thumbnails) > 0 {
		status = "ready_for_fulfillment"
		update = "Checkout received. Product images are ready for fulfillment."
	}
	return CheckoutReceipt{OrderID: orderID, CustomerEmail: email, ProductSKU: sku, Status: status, Thumbnails: thumbnails, CustomerUpdate: update}
}

func writeJSON(w http.ResponseWriter, status int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(value)
}
