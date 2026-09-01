package main

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestResizeRequestBoundary(t *testing.T) {
	tests := []struct {
		name        string
		status      int
		envelope    any
		wantID      string
		wantErrCode string
	}{
		{name: "stored thumbnail", status: http.StatusOK, envelope: map[string]any{"ok": true, "data": map[string]string{"id": "img_640", "url": "https://cdn.example/img_640.webp"}, "metadata": map[string]any{}}, wantID: "img_640"},
		{name: "business rejection preserves code", status: http.StatusBadRequest, envelope: map[string]any{"ok": false, "error": map[string]string{"code": "rejected", "message": "image input rejected"}, "metadata": map[string]any{}}, wantErrCode: "rejected"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				if r.Method != http.MethodPost || r.URL.Path != imageProcessPath {
					t.Fatalf("request = %s %s", r.Method, r.URL.Path)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Fatalf("authorization header missing")
				}
				if r.Header.Get("Idempotency-Key") != "checkout-ord_1042-0-640x640-webp" {
					t.Fatalf("idempotency key = %q", r.Header.Get("Idempotency-Key"))
				}
				if got := r.Header.Get("Content-Type"); got != "application/json" {
					t.Fatalf("content type = %q", got)
				}
				var body map[string]json.RawMessage
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Fatal(err)
				}
				for _, field := range []string{"image", "ops", "format", "store"} {
					if _, present := body[field]; !present {
						t.Fatalf("request field %q is absent", field)
					}
				}
				for _, field := range []string{"width", "height", "fit", "enlarge"} {
					if _, present := body[field]; present {
						t.Fatalf("unsupported top-level field %q is present", field)
					}
				}
				var image map[string]string
				if err := json.Unmarshal(body["image"], &image); err != nil {
					t.Fatal(err)
				}
				wantImage := base64.StdEncoding.EncodeToString([]byte("jpeg-bytes"))
				if image["base64"] != wantImage {
					t.Fatalf("image = %#v, want base64 %q", image, wantImage)
				}
				if got := string(body["ops"]); got != `[{"op":"resize","params":{"fit":"cover","height":640,"width":640}}]` {
					t.Fatalf("ops = %s", got)
				}
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(test.status)
				_ = json.NewEncoder(w).Encode(test.envelope)
			}))
			defer server.Close()

			client := NewInfraiImageClient(server.URL, "test-key", server.Client())
			image, err := client.Resize(context.Background(), ResizeRequest{
				Image: []byte("jpeg-bytes"), Filename: "shoe.jpg", Width: 640, Height: 640,
				Fit: "cover", Format: "webp", Store: true,
				IdempotencyKey: "checkout-ord_1042-0-640x640-webp",
			})
			if test.wantErrCode != "" {
				apiErr, ok := err.(*InfraiError)
				if !ok || apiErr.Code != test.wantErrCode || apiErr.HTTPStatus != test.status {
					t.Fatalf("error = %#v", err)
				}
				return
			}
			if err != nil || image.ID != test.wantID {
				t.Fatalf("image = %#v, error = %v", image, err)
			}
		})
	}
}
