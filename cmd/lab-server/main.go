package main

import (
	"context"
	"errors"
	"godatabase/internal/experiment"
	"godatabase/internal/metrics"
	"godatabase/internal/server"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"
)

func main() {
	address := os.Getenv("ALGODB_ADDR")
	if address == "" {
		address = ":8080"
	}
	origin := os.Getenv("ALGODB_FRONTEND_ORIGIN")
	if origin == "" {
		origin = "http://localhost:3000"
	}
	handler := server.New(server.Config{Runner: experiment.NewRunner(nil), Complexity: metrics.DefaultCatalog(), AllowedOrigin: origin})
	api := &http.Server{Addr: address, Handler: handler, ReadHeaderTimeout: 5 * time.Second, ReadTimeout: 35 * time.Second, WriteTimeout: 35 * time.Second, IdleTimeout: 60 * time.Second}
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	go func() {
		<-ctx.Done()
		shutdownCtx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
		defer cancel()
		if err := api.Shutdown(shutdownCtx); err != nil {
			log.Printf("API shutdown failed: %v", err)
		}
	}()
	log.Printf("AlgoDB Lab API listening on %s", address)
	if err := api.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
		log.Fatal(err)
	}
}
