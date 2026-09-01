package main

import "testing"

func TestDecideReceipt(t *testing.T) {
	tests := []struct {
		name       string
		thumbnails []Thumbnail
		wantStatus string
	}{
		{name: "no completed thumbnail needs review", wantStatus: "thumbnail_review"},
		{name: "completed thumbnail releases fulfillment", thumbnails: []Thumbnail{{ImageID: "img_01", URL: "https://cdn.example/item.webp", Width: 640, Height: 640}}, wantStatus: "ready_for_fulfillment"},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			receipt := decideReceipt("ord_1042", "buyer@example.com", "SKU-RED-42", test.thumbnails)
			if receipt.Status != test.wantStatus {
				t.Fatalf("status = %q, want %q", receipt.Status, test.wantStatus)
			}
			if receipt.OrderID != "ord_1042" || receipt.ProductSKU != "SKU-RED-42" {
				t.Fatalf("receipt lost order identity: %#v", receipt)
			}
		})
	}
}
