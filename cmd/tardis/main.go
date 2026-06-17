package main

import (
	"context"
	"encoding/json"
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

	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"tardis/internal/api"
	"tardis/internal/assets"
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
		case "status":
			runStatus()
			return
		case "stop":
			runStop()
			return
		case "pause":
			runPause()
			return
		case "resume":
			runResume()
			return
		case "--version", "-v":
			fmt.Printf("Tardis %s\n", Version)
			return
		case "--help", "-h":
			printUsage()
			return
		default:
			fmt.Printf("❌ Unknown command: %s\n\n", os.Args[1])
			printUsage()
			os.Exit(1)
		}
	}

	printUsage()
}

func printUsage() {
	fmt.Println("🚀 Tardis Cloud Storage Proxy Daemon")
	fmt.Println("\nUsage:")
	fmt.Println("  tardis init          Launch full-screen setup wizard")
	fmt.Println("  tardis start         Start the daemon (uses default ~/.tardis/config.yml)")
	fmt.Println("  tardis start -c ...  Start with a custom config path")
	fmt.Println("  tardis status        Check the daemon running status")
	fmt.Println("  tardis stop          Stop the running daemon gracefully")
	fmt.Println("  tardis pause         Pause dispatching new tasks")
	fmt.Println("  tardis resume        Resume dispatching tasks")
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

	var showGuide bool

	for {
		showGuide = false
		form2 := huh.NewForm(
			huh.NewGroup(
				huh.NewNote().
					Title("🔑 Google Drive Setup").
					Description(fmt.Sprintf(
						"Tardis connects to Google Drive securely using a GCP Service Account.\n\n"+
							"Follow these 3 quick steps to set it up:\n"+
							"1. Enable Google Drive API in your GCP project:\n"+
							"   👉 %s\n"+
							"2. Create a Service Account in IAM, generate a JSON key, and download it.\n"+
							"3. In Google Drive, create a folder and share it with your Service Account's email.",
						gcpSetupURL,
					)),
				huh.NewInput().
					Title("Absolute path to credentials.json:").
					Value(&credPath),
				huh.NewConfirm().
					Title("How do I get a credentials.json file?").
					Value(&showGuide).
					Affirmative("View Guide").
					Negative("I have it"),
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

		if showGuide {
			runGuideViewer()
			continue
		}

		if !confirm {
			fmt.Println("Setup cancelled.")
			os.Exit(1)
		}

		// Validate path only when trying to complete setup without viewing guide
		if credPath == "" {
			fmt.Println("❌ Error: path to credentials.json cannot be empty. Press Enter to try again.")
			var dummy string
			fmt.Scanln(&dummy)
			continue
		}
		if _, err := os.Stat(credPath); os.IsNotExist(err) {
			fmt.Printf("❌ Error: file not found at '%s'. Press Enter to try again.\n", credPath)
			var dummy string
			fmt.Scanln(&dummy)
			continue
		}

		break
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

	// Write PID file and check duplication
	if err := writePIDFile(); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}
	defer removePIDFile()

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
	handler := api.NewHandler(db, scheduler)
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

// getPIDFilePath returns the path to the PID file.
func getPIDFilePath() string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".tardis", "tardis.pid")
	}
	return "./tardis.pid"
}

// writePIDFile writes the current PID to the PID file if it is not already running.
func writePIDFile() error {
	pidPath := getPIDFilePath()
	if data, err := os.ReadFile(pidPath); err == nil {
		var pid int
		if _, scanErr := fmt.Sscanf(string(data), "%d", &pid); scanErr == nil {
			if isProcessRunning(pid) {
				return fmt.Errorf("tardis daemon is already running (PID: %d)", pid)
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(pidPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

// removePIDFile removes the PID file.
func removePIDFile() {
	_ = os.Remove(getPIDFilePath())
}

// isProcessRunning checks if the process with the given PID is running.
func isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return isProcessAlive(proc)
}

// isProcessAlive returns true if the process is alive.
func isProcessAlive(p *os.Process) bool {
	if runtime.GOOS == "windows" {
		err := p.Signal(os.Interrupt)
		return err == nil || !errors.Is(err, os.ErrProcessDone)
	}
	err := p.Signal(syscall.Signal(0))
	return err == nil
}

// getDaemonPort loads the port from default config or returns 8080.
func getDaemonPort() int {
	cfgPath := getDefaultConfigPath()
	if cfg, err := config.Load(cfgPath); err == nil {
		if cfg.Server.Port != 0 {
			return cfg.Server.Port
		}
	}
	return 8080
}

func runStatus() {
	port := getDaemonPort()
	url := fmt.Sprintf("http://localhost:%d/status", port)

	resp, err := http.Get(url)
	if err != nil {
		pidPath := getPIDFilePath()
		if _, statErr := os.Stat(pidPath); statErr == nil {
			fmt.Println("⚠️  PID file exists but daemon is not responding. (Status: Stale)")
		} else {
			fmt.Println("ℹ️  Tardis daemon is not running.")
		}
		return
	}
	defer resp.Body.Close()

	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		fmt.Printf("❌ Failed to parse status response: %v\n", err)
		return
	}

	fmt.Printf("● Tardis Daemon Status: %v (PID: %v)\n", res["status"], res["pid"])
}

func runStop() {
	port := getDaemonPort()
	url := fmt.Sprintf("http://localhost:%d/stop", port)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("❌ Failed to contact daemon: %v (Is it running?)\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("🛑 Shutdown signal sent to Tardis daemon.")
	} else {
		fmt.Printf("❌ Shutdown request failed with status: %d\n", resp.StatusCode)
	}
}

func runPause() {
	port := getDaemonPort()
	url := fmt.Sprintf("http://localhost:%d/pause", port)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("❌ Failed to contact daemon: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("⏸️  Tardis daemon has been paused.")
	} else {
		fmt.Printf("❌ Pause request failed with status: %d\n", resp.StatusCode)
	}
}

func runResume() {
	port := getDaemonPort()
	url := fmt.Sprintf("http://localhost:%d/resume", port)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("❌ Failed to contact daemon: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Println("▶️  Tardis daemon has been resumed.")
	} else {
		fmt.Printf("❌ Resume request failed with status: %d\n", resp.StatusCode)
	}
}

type guideModel struct {
	viewport viewport.Model
	content  string
	ready    bool
}

func newGuideModel(content string) guideModel {
	return guideModel{
		content: content,
	}
}

func (m guideModel) Init() tea.Cmd {
	return nil
}

func (m guideModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmds []tea.Cmd
	var cmd tea.Cmd

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "q", "ctrl+c", "esc":
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		headerHeight := 2
		footerHeight := 2
		verticalMarginHeight := headerHeight + footerHeight

		if !m.ready {
			m.viewport = viewport.New(msg.Width, msg.Height-verticalMarginHeight)
			m.viewport.YPosition = headerHeight
			m.viewport.SetContent(m.content)
			m.ready = true
		} else {
			m.viewport.Width = msg.Width
			m.viewport.Height = msg.Height - verticalMarginHeight
		}
	}

	m.viewport, cmd = m.viewport.Update(msg)
	cmds = append(cmds, cmd)

	return m, tea.Batch(cmds...)
}

func (m guideModel) View() string {
	if !m.ready {
		return "\n  Initializing guide..."
	}
	header := "📖 Google Drive GCP Setup Guide (Press 'q' or 'Esc' to exit)\n-----------------------------------------------------------"
	footer := fmt.Sprintf("-----------------------------------------------------------\nScroll: ↑/↓/PgUp/PgDn | %3.f%%", m.viewport.ScrollPercent()*100)
	return fmt.Sprintf("%s\n%s\n%s", header, m.viewport.View(), footer)
}

func runGuideViewer() {
	p := tea.NewProgram(newGuideModel(assets.GCPGuideText), tea.WithAltScreen())
	if _, err := p.Run(); err != nil {
		fmt.Printf("Error running guide viewer: %v\n", err)
	}
}

