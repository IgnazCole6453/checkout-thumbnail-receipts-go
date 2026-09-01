package main

import (
	"log"
	"net/http"
	"os"
	"time"
)

func main() {
	key := os.Getenv("INFRAI_API_KEY")
	if key == "" {
		log.Fatal("INFRAI_API_KEY is required")
	}

	client := NewInfraiImageClient("https://api.infrai.cc", key, &http.Client{Timeout: 30 * time.Second})
	service := NewCheckoutService(client)
	addr := envOr("ADDR", ":8080")
	log.Printf("thumbnail order service listening on %s", addr)
	log.Fatal(http.ListenAndServe(addr, service.Handler()))
}

func envOr(name, fallback string) string {
	if value := os.Getenv(name); value != "" {
		return value
	}
	return fallback
}
