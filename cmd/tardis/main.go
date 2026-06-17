package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"syscall"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"tardis/internal/api"
	"tardis/internal/config"
	"tardis/internal/queue"
	"tardis/internal/storage"
	"tardis/internal/worker"
)

var Version = "dev"

func getDefaultConfigPath() string {
	// Priority 1: User-level config (created by `tardis init`)
	if home, err := os.UserHomeDir(); err == nil {
		userConfig := filepath.Join(home, ".tardis", "config.yml")
		if _, err := os.Stat(userConfig); err == nil {
			return userConfig
		}
	}

	// Priority 2: System-level config (created by `install.sh` or `make install`)
	systemConfig := "/etc/tardis/tardis.yml"
	if _, err := os.Stat(systemConfig); err == nil {
		return systemConfig
	}

	// Fallback: user-level path (will trigger "run tardis init" message if missing)
	home, err := os.UserHomeDir()
	if err != nil {
		return "./tardis.yml"
	}
	return filepath.Join(home, ".tardis", "config.yml")
}

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

	runStart(os.Args[1:])
}

func printUsage() {
	fmt.Println("🚀 Tardis Cloud Storage Proxy Daemon")
	fmt.Println("\nUsage:")
	fmt.Println("  tardis init          Launch full-screen setup wizard")
	fmt.Println("  tardis start         Start the daemon (uses default ~/.tardis/config.yml)")
	fmt.Println("  tardis start -c ...  Start with a custom config path")
	fmt.Println("  tardis --version     Show version")
	fmt.Println("  tardis --help        Show help")
}

func runInit() {
	var provider string
	var credPath string
	var rootDir string = "tardis"
	var confirm bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("✨ Tardis Setup Wizard ✨").
				Description("Welcome! Let's configure your decentralized cloud storage gateway.\nPress Enter to continue."),
			huh.NewSelect[string]().
				Title("Choose your cloud storage provider:").
				Options(
					huh.NewOption("Google Drive", "Google Drive"),
				).
				Value(&provider),
		),
	).WithProgramOptions(tea.WithAltScreen())

	if err := form.Run(); err != nil {
		fmt.Println("Wizard aborted.")
		os.Exit(1)
	}

	if provider != "Google Drive" {
		fmt.Printf("❌ Sorry, %s is not supported in this beta version. Exiting.\n", provider)
		os.Exit(1)
	}

	// Try to automatically open the Google Drive API setup page in the browser
	gcpSetupURL := "https://console.cloud.google.com/apis/library/drive.googleapis.com"
	_ = openBrowser(gcpSetupURL)

	form2 := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("🔑 Google Drive Setup").
				Description(fmt.Sprintf(
					"Tardis needs your GCP Service Account JSON key to operate independently.\n\n"+
						"Please enable Google Drive API in your GCP project:\n"+
						"👉 %s\n\n"+
						"(We have tried opening this link in your default browser.)",
					gcpSetupURL,
				)),
			huh.NewInput().
				Title("Absolute path to credentials.json:").
				Value(&credPath).
				Validate(func(str string) error {
					if str == "" {
						return errors.New("path cannot be empty")
					}
					if _, err := os.Stat(str); os.IsNotExist(err) {
						return fmt.Errorf("file not found: %s", str)
					}
					return nil
				}),
			huh.NewInput().
				Title("Google Drive Root Directory Name (default: tardis):").
				Value(&rootDir).
				Placeholder("tardis"),
			huh.NewConfirm().
				Title("Ready to save configuration?").
				Value(&confirm),
		),
	).WithProgramOptions(tea.WithAltScreen())

	if err := form2.Run(); err != nil {
		fmt.Println("Wizard aborted.")
		os.Exit(1)
	}

	if !confirm {
		fmt.Println("Setup cancelled.")
		os.Exit(1)
	}

	if rootDir == "" {
		rootDir = "tardis"
	}

	// Save Configuration
	cfgPath := getDefaultConfigPath()
	cfgDir := filepath.Dir(cfgPath)

	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		fmt.Printf("\n❌ Failed to create config directory at %s: %v\n", cfgDir, err)
		os.Exit(1)
	}

	dataDir := filepath.Join(cfgDir, "data")
	configTemplate := fmt.Sprintf(`server:
  port: 8080
  data_dir: "%s"

storage:
  provider: "google_drive"
  google_drive:
    credentials_path: "%s"
    rate_limit_per_second: 10
    retry_max_attempts: 5
    root_dir: "%s"

workers:
  pool_size: 4
  chunk_size_mb: 10
`, dataDir, credPath, rootDir)

	if err := os.WriteFile(cfgPath, []byte(configTemplate), 0644); err != nil {
		fmt.Printf("\n❌ Failed to save config file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Success! Tardis is ready to run.\n")
	fmt.Printf("📂 Configuration saved to: %s\n", cfgPath)
	fmt.Println("\nYou can now start the daemon by running:")
	fmt.Println("👉  tardis start")
}

func runStart(args []string) {
	defaultCfg := getDefaultConfigPath()
	startCmd := flag.NewFlagSet("start", flag.ExitOnError)
	configPath := startCmd.String("c", defaultCfg, "path to config file")
	startCmd.Parse(args)

	// Check if config exists
	if _, err := os.Stat(*configPath); os.IsNotExist(err) {
		fmt.Printf("❌ Config file not found at %s\n", *configPath)
		fmt.Println("💡 Please run 'tardis init' first to generate a configuration.")
		os.Exit(1)
	}

	fmt.Println("🚀 Tardis Cloud Storage Proxy Daemon starting...")

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
		provider, err = storage.NewGoogleDriveAdapter(context.Background(), cfg.Storage.GoogleDrive.CredentialsPath, cfg.Storage.GoogleDrive.RootDir, db)
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

// openBrowser attempts to open the specified URL in the default browser based on the OS.
func openBrowser(url string) error {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	case "darwin":
		cmd = exec.Command("open", url)
	case "linux":
		cmd = exec.Command("xdg-open", url)
	default:
		return fmt.Errorf("unsupported platform")
	}
	return cmd.Start()
}

