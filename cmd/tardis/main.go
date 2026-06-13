package main

import (
	"fmt"
	"log"
	"net/http"

	"tardis/internal/config"
	"tardis/internal/queue"
)

func main() {
	fmt.Println("Tardis Cloud Storage Proxy Demon starting...")

	// 1. Load Configuration
	cfg, err := config.Load("tardis.yml")
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 2. Initialize Database (Queue & Cache)
	db, err := queue.Open(cfg.Server.DataDir)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()

	log.Printf("Configuration loaded. Workers pool size: %d", cfg.Workers.PoolSize)
	log.Printf("SQLite DB initialized at %s/tardis.db", cfg.Server.DataDir)

	http.HandleFunc("/health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	log.Printf("Server is running on port %d", cfg.Server.Port)
	if err := http.ListenAndServe(fmt.Sprintf(":%d", cfg.Server.Port), nil); err != nil {
		log.Fatalf("Server failed: %v", err)
	}
}
