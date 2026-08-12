package main

import (
	"context"
	"log"
	"net/http"
	"time"

	"github.com/financial-aggregator/ledger/internal/config"
	"github.com/financial-aggregator/ledger/internal/db"
	"github.com/financial-aggregator/ledger/internal/handler"
)

func main() {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	// Validate all required environment variables are present
	cfg.Validate()

	store, err := db.Connect(ctx, cfg)
	if err != nil {
		log.Fatalf("failed to connect to db: %v", err)
	}
	defer store.Close()

	h := handler.NewHandler(store, cfg)
	router := h.Routes()

	addr := ":" + cfg.Port
	log.Printf("ledger service starting on %s", addr)
	if err := http.ListenAndServe(addr, router); err != nil {
		log.Fatalf("server failed: %v", err)
	}
}
