package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"path/filepath"
	"strings"
	"syscall"
	"time"

	"tardis/internal/api"
	"tardis/internal/config"
	"tardis/internal/queue"
	"tardis/internal/storage"
	"tardis/internal/worker"
)

var Version = "v0.1.0-beta.2"

func main() {
	if len(os.Args) > 1 {
		switch os.Args[1] {
		case "init":
			runInit()
			return
		case "start":
			runStart(os.Args[2:])
			return
		case "--version", "-v":
			fmt.Printf("Tardis %s\n", Version)
			return
		case "--help", "-h":
			printUsage()
			return
		}
	}

	// Fallback to start if no known subcommand is provided
	runStart(os.Args[1:])
}

func printUsage() {
	fmt.Println("Tardis Cloud Storage Proxy Daemon")
	fmt.Println("Usage:")
	fmt.Println("  tardis init          Interactive setup to connect your cloud provider")
	fmt.Println("  tardis start -c ...  Start the proxy daemon")
	fmt.Println("  tardis [flags]       Fallback for starting the daemon directly")
	fmt.Println("  tardis --version     Print version information")
	fmt.Println("  tardis --help        Print this message")
}

func runInit() {
	fmt.Println("=== Tardis Initialization ===")
	reader := bufio.NewReader(os.Stdin)

	fmt.Println("\n1. Select Cloud Provider:")
	fmt.Println("   [1] Google Drive")
	fmt.Print("Enter choice (default: 1): ")
	choice, _ := reader.ReadString('\n')
	choice = strings.TrimSpace(choice)
	
	if choice != "1" && choice != "" {
		fmt.Println("Currently only Google Drive [1] is supported. Defaulting to Google Drive.")
	}

	fmt.Println("\n2. Google Drive Setup")
	fmt.Println("Tardis requires your own Google Cloud Service Account credentials to operate independently.")
	fmt.Println("Please create a Service Account in GCP and download the JSON key file.")
	
	fmt.Print("\nEnter the absolute path to your credentials.json: ")
	credPath, _ := reader.ReadString('\n')
	credPath = strings.TrimSpace(credPath)

	if credPath == "" {
		fmt.Println("Error: Credentials path is required.")
		os.Exit(1)
	}

	// Basic file existence check
	if _, err := os.Stat(credPath); os.IsNotExist(err) {
		fmt.Printf("Warning: File not found at %s. Please ensure it exists before starting the daemon.\n", credPath)
	}

	fmt.Print("\nEnter the absolute path where tardis.yml should be saved (default: ./tardis.yml): ")
	cfgPath, _ := reader.ReadString('\n')
	cfgPath = strings.TrimSpace(cfgPath)
	if cfgPath == "" {
		cfgPath = "./tardis.yml"
	}

	// Generate basic config
	configTemplate := fmt.Sprintf(`server:
  port: 8080
  data_dir: "%s"

storage:
  provider: "google_drive"
  google_drive:
    credentials_path: "%s"
    rate_limit_per_second: 10
    retry_max_attempts: 5

workers:
  pool_size: 4
  chunk_size_mb: 10
`, filepath.Join(filepath.Dir(cfgPath), "data"), credPath)

	err := os.WriteFile(cfgPath, []byte(configTemplate), 0644)
	if err != nil {
		fmt.Printf("Failed to write config file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nInitialization complete! Config saved to %s\n", cfgPath)
	fmt.Printf("You can now start Tardis using:\n  tardis start -c %s\n", cfgPath)
}

func runStart(args []string) {
	startCmd := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := startCmd.String("c", "tardis.yml", "path to config file")
	startCmd.Parse(args)

	fmt.Println("Tardis Cloud Storage Proxy Daemon starting...")

	// 1. Setup root context for graceful shutdown
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2. Load Configuration
	cfg, err := config.Load(*configPath)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 3. Initialize Database (Queue & Cache)
	db, err := queue.Open(cfg.Server.DataDir)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	log.Printf("SQLite DB initialized at %s/tardis.db", cfg.Server.DataDir)

	// 4. Initialize Storage Provider
	var provider storage.Provider
	if cfg.Storage.Provider == "google_drive" {
		provider, err = storage.NewGoogleDriveAdapter(context.Background(), cfg.Storage.GoogleDrive.CredentialsPath, db)
		if err != nil {
			log.Fatalf("Failed to initialize Google Drive adapter: %v", err)
		}
	} else {
		log.Fatalf("Unsupported storage provider: %s", cfg.Storage.Provider)
	}

	// 5. Initialize Task Channel
	taskChan := make(chan *queue.Task, cfg.Workers.PoolSize*2)

	// 6. Start Worker Pool
	pool := worker.NewPool(
		db,
		provider,
		taskChan,
		cfg.Workers.PoolSize,
		cfg.Storage.GoogleDrive.RateLimitPerSec,
		cfg.Storage.GoogleDrive.RetryMaxAttempts,
	)
	pool.Start(ctx)

	// 7. Start Scheduler
	scheduler := worker.NewScheduler(db, taskChan)
	go scheduler.Start(ctx)

	// 8. Setup HTTP API
	mux := http.NewServeMux()
	handler := api.NewHandler(db)
	handler.RegisterRoutes(mux)

	mux.HandleFunc("GET /health", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		w.Write([]byte("OK"))
	})

	srv := &http.Server{
		Addr:    fmt.Sprintf(":%d", cfg.Server.Port),
		Handler: mux,
	}

	// 9. Start HTTP Server in background
	go func() {
		log.Printf("Server is running on port %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// 10. Wait for SIGINT/SIGTERM
	<-ctx.Done()
	log.Println("\nReceived shutdown signal. Commencing graceful shutdown...")

	// 11. Graceful Shutdown Sequence
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	// Stop accepting new HTTP requests
	if err := srv.Shutdown(shutdownCtx); err != nil {
		log.Printf("HTTP server shutdown error: %v", err)
	} else {
		log.Println("HTTP server stopped.")
	}

	// Wait for ongoing worker tasks to complete (Wait implicitly waits because context is canceled, but waitgroup tracks completion)
	log.Println("Waiting for workers to finish current chunks...")
	pool.Wait()
	log.Println("Workers finished.")

	// Database is deferred to close at the end of main()
	log.Println("Tardis shutdown gracefully. Bye!")
}

