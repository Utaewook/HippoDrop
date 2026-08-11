package core

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"time"

	"hippodrop/internal/api"
	"hippodrop/internal/config"
	"hippodrop/internal/queue"
	"hippodrop/internal/storage"
	"hippodrop/internal/worker"
)

// Run assembles the queue, storage provider, worker pool, scheduler,
// webhook sender, and HTTP API, then blocks until ctx is canceled and
// drains in-flight work before returning.
func Run(ctx context.Context, cfg *config.Config) error {
	db, err := queue.Open(cfg.Server.DataDir)
	if err != nil {
		return fmt.Errorf("failed to initialize database: %w", err)
	}
	defer db.Close()
	log.Printf("SQLite DB initialized at %s/hippodrop.db", cfg.Server.DataDir)

	var provider storage.Provider
	if cfg.Storage.Provider == "google_drive" {
		provider, err = storage.NewGoogleDriveAdapter(ctx, cfg.Storage.GoogleDrive.CredentialsPath, cfg.Storage.GoogleDrive.TokenPath, cfg.Storage.GoogleDrive.RootDir, db)
		if err != nil {
			return fmt.Errorf("failed to initialize Google Drive adapter: %w", err)
		}
	} else {
		return fmt.Errorf("unsupported storage provider: %s", cfg.Storage.Provider)
	}

	taskChan := make(chan *queue.Task, cfg.Workers.PoolSize*2)

	pool := worker.NewPool(
		db,
		provider,
		taskChan,
		cfg.Workers.PoolSize,
		cfg.Storage.GoogleDrive.RateLimitPerSec,
		cfg.Storage.GoogleDrive.RetryMaxAttempts,
	)
	pool.Start(ctx)

	scheduler := worker.NewScheduler(db, taskChan)
	go scheduler.Start(ctx)

	webhookSender := worker.NewWebhookSender(db, cfg.Server.APIKey)
	go webhookSender.Start(ctx)

	mux := http.NewServeMux()
	handler := api.NewHandler(db, scheduler, cfg.Server.APIKey)
	handler.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	bindAddr := "127.0.0.1"
	if cfg.Server.APIKey != "" {
		bindAddr = "0.0.0.0"
	}

	srv := &http.Server{
		Addr:    fmt.Sprintf("%s:%d", bindAddr, cfg.Server.Port),
		Handler: mux,
	}

	go func() {
		log.Printf("Server is running on port %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	<-ctx.Done()
	log.Println("\nReceived shutdown signal. Stopping daemon...")

	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop accepting new HTTP requests
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	} else {
		log.Println("HTTP server stopped.")
	}

	// Wait for ongoing worker tasks to complete
	log.Println("Waiting for workers to finish current chunks...")
	pool.Wait()
	log.Println("Workers finished.")

	log.Println("HippoDrop daemon stopped.")
	return nil
}
