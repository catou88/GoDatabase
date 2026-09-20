package main

import (
	"log"
	"net/http"
	"os"

	"godatabase/internal/experiment"
	"godatabase/internal/metrics"
	"godatabase/internal/server"
)

func main() {
	address := os.Getenv("ALGODB_ADDR")
	if address == "" {
		address = ":8080"
	}
	allowedOrigin := os.Getenv("ALGODB_FRONTEND_ORIGIN")
	if allowedOrigin == "" {
		allowedOrigin = "http://localhost:3000,http://127.0.0.1:3000"
	}
	handler := server.New(server.Config{Runner: experiment.BasicRunner{Complexity: metrics.DefaultCatalog()}, AllowedOrigin: allowedOrigin})
	log.Printf("AlgoDB Lab API listening on %s", address)
	if err := http.ListenAndServe(address, handler); err != nil {
		log.Fatal(err)
	}
}
