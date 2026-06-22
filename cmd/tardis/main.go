package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"runtime"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"
	"time"

	"github.com/charmbracelet/bubbles/key"
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

func getProjectDir(projectName string) string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".tardis", "projects", projectName)
	}
	return fmt.Sprintf("./.tardis/projects/%s", projectName)
}

func getProjectConfigPath(projectName string) string {
	return filepath.Join(getProjectDir(projectName), "config.yml")
}

func getPIDFilePath(projectName string) string {
	return filepath.Join(getProjectDir(projectName), "tardis.pid")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "--version", "-v":
		fmt.Printf("Tardis %s\n", Version)
		return
	case "--help", "-h":
		printUsage()
		return
	case "ls":
		runLs()
		return
	}

	if len(os.Args) < 3 {
		fmt.Printf("❌ Missing project name.\n\n")
		printUsage()
		os.Exit(1)
	}

	projectName := os.Args[2]

	if strings.TrimSpace(projectName) == "" {
		fmt.Println("❌ Project name cannot be empty.")
		printUsage()
		os.Exit(1)
	}

	switch command {
	case "init":
		runInit(projectName)
	case "start":
		runStart(projectName)
	case "status":
		runStatus(projectName)
	case "stop":
		runStop(projectName)
	case "pause":
		runPause(projectName)
	case "resume":
		runResume(projectName)
	case "rm", "remove":
		runRm(projectName)
	default:
		fmt.Printf("❌ Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("🚀 Tardis Cloud Storage Proxy Daemon")
	fmt.Println("\nUsage:")
	fmt.Println("  tardis init <project>         Launch setup wizard for a new project")
	fmt.Println("  tardis start <project> [-d]   Start the daemon (use -d for background)")
	fmt.Println("  tardis ls [-a] [-l]           List daemons (-a: all, -l: details)")
	fmt.Println("  tardis status <project>       Check the daemon running status")
	fmt.Println("  tardis stop <project>         Stop the running daemon gracefully")
	fmt.Println("  tardis pause <project>        Pause dispatching new tasks")
	fmt.Println("  tardis resume <project>       Resume dispatching tasks")
	fmt.Println("  tardis rm <project>           Remove a stopped project completely")
	fmt.Println("  tardis --version              Show version")
	fmt.Println("  tardis --help                 Show help")
}

func getNextAvailablePort() int {
	home, err := os.UserHomeDir()
	if err != nil {
		return 8080
	}
	projectsDir := filepath.Join(home, ".tardis", "projects")
	
	highestPort := 8079
	entries, err := os.ReadDir(projectsDir)
	if err != nil {
		return 8080 // directory might not exist yet
	}
	
	for _, entry := range entries {
		if entry.IsDir() {
			cfgPath := filepath.Join(projectsDir, entry.Name(), "config.yml")
			if cfg, err := config.Load(cfgPath); err == nil {
				if cfg.Server.Port > highestPort {
					highestPort = cfg.Server.Port
				}
			}
		}
	}
	return highestPort + 1
}

func runInit(projectName string) {
	var provider string
	var credPath string
	var portStr = fmt.Sprintf("%d", getNextAvailablePort())
	var rootDir string = projectName
	var confirm bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("✨ Tardis Setup Wizard ✨").
				Description(fmt.Sprintf("Welcome! Let's configure your project '%s'.\nPress Enter to continue.", projectName)),
			huh.NewSelect[string]().
				Title("Choose your cloud storage provider:").
				Options(
					huh.NewOption("Google Drive", "Google Drive"),
				).
				Value(&provider),
		),
	).WithKeyMap(arrowKeyMap()).WithProgramOptions(tea.WithAltScreen())

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

	if !runCredSetup(&credPath, &rootDir, &portStr, &confirm, gcpSetupURL) {
		fmt.Println("Wizard aborted.")
		os.Exit(1)
	}

	if !confirm {
		fmt.Println("Setup cancelled.")
		os.Exit(1)
	}

	if rootDir == "" {
		rootDir = projectName
	}
	
	port, err := strconv.Atoi(portStr)
	if err != nil || port <= 0 {
		fmt.Printf("❌ Invalid port number: %s\n", portStr)
		os.Exit(1)
	}

	// Save Configuration
	cfgPath := getProjectConfigPath(projectName)
	cfgDir := filepath.Dir(cfgPath)

	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		fmt.Printf("\n❌ Failed to create config directory at %s: %v\n", cfgDir, err)
		os.Exit(1)
	}

	dataDir := filepath.Join(cfgDir, "data")
	configTemplate := fmt.Sprintf(`server:
  port: %d
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
`, port, dataDir, credPath, rootDir)

	if err := os.WriteFile(cfgPath, []byte(configTemplate), 0644); err != nil {
		fmt.Printf("\n❌ Failed to save config file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\n✅ Success! Project '%s' is ready to run.\n", projectName)
	fmt.Printf("📂 Configuration saved to: %s\n", cfgPath)
	fmt.Println("\nYou can now start the daemon by running:")
	fmt.Printf("👉  tardis start %s\n", projectName)
}

func runStart(projectName string) {
	detach := false
	for _, arg := range os.Args[3:] {
		if arg == "-d" {
			detach = true
		}
	}

	if detach {
		exe, _ := os.Executable()
		cmd := exec.Command(exe, "start", projectName)
		
		logPath := filepath.Join(getProjectDir(projectName), "daemon.log")
		if err := os.MkdirAll(filepath.Dir(logPath), 0755); err != nil {
			fmt.Printf("❌ Failed to create log directory: %v\n", err)
			os.Exit(1)
		}
		
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Printf("❌ Failed to open daemon.log: %v\n", err)
			os.Exit(1)
		}
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		if err := cmd.Start(); err != nil {
			fmt.Printf("❌ Failed to start daemon in background: %v\n", err)
			os.Exit(1)
		}

		fmt.Printf("✅ Tardis daemon started in background (PID: %d)\n", cmd.Process.Pid)
		fmt.Printf("📂 Logs: %s\n", logPath)
		return
	}

	configPath := getProjectConfigPath(projectName)

	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("❌ Config file not found at %s\n", configPath)
		fmt.Printf("💡 Please run 'tardis init %s' first to generate a configuration.\n", projectName)
		os.Exit(1)
	}

	// Write PID file and check duplication
	if err := writePIDFile(projectName); err != nil {
		fmt.Printf("❌ %v\n", err)
		os.Exit(1)
	}
	defer removePIDFile(projectName)

	fmt.Printf("🚀 Tardis Daemon starting for project '%s'...\n", projectName)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2. Load Configuration
	cfg, err := config.Load(configPath)
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

	// Wait for ongoing worker tasks to complete
	log.Println("Waiting for workers to finish current chunks...")
	pool.Wait()
	log.Println("Workers finished.")

	log.Println("Tardis shutdown gracefully. Bye!")
}

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

func writePIDFile(projectName string) error {
	pidPath := getPIDFilePath(projectName)
	if data, err := os.ReadFile(pidPath); err == nil {
		var pid int
		if _, scanErr := fmt.Sscanf(string(data), "%d", &pid); scanErr == nil {
			if isProcessRunning(pid) {
				return fmt.Errorf("tardis daemon is already running for project '%s' (PID: %d)", projectName, pid)
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(pidPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

func removePIDFile(projectName string) {
	_ = os.Remove(getPIDFilePath(projectName))
}

func isProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return isProcessAlive(proc)
}

func isProcessAlive(p *os.Process) bool {
	if runtime.GOOS == "windows" {
		err := p.Signal(os.Interrupt)
		return err == nil || !errors.Is(err, os.ErrProcessDone)
	}
	err := p.Signal(syscall.Signal(0))
	return err == nil
}

func getDaemonPort(projectName string) (int, error) {
	cfgPath := getProjectConfigPath(projectName)
	if cfg, err := config.Load(cfgPath); err == nil {
		if cfg.Server.Port != 0 {
			return cfg.Server.Port, nil
		}
	}
	return 0, fmt.Errorf("could not load config for project %s", projectName)
}

func runStatus(projectName string) {
	port, err := getDaemonPort(projectName)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return
	}
	url := fmt.Sprintf("http://localhost:%d/status", port)

	resp, err := http.Get(url)
	if err != nil {
		pidPath := getPIDFilePath(projectName)
		if _, statErr := os.Stat(pidPath); statErr == nil {
			fmt.Println("⚠️  PID file exists but daemon is not responding. (Status: Stale)")
		} else {
			fmt.Printf("ℹ️  Tardis daemon for project '%s' is not running.\n", projectName)
		}
		return
	}
	defer resp.Body.Close()

	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		fmt.Printf("❌ Failed to parse status response: %v\n", err)
		return
	}

	fmt.Printf("● Project '%s' Status: %v (PID: %v)\n", projectName, res["status"], res["pid"])
}

func runStop(projectName string) {
	port, err := getDaemonPort(projectName)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return
	}
	url := fmt.Sprintf("http://localhost:%d/stop", port)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("❌ Failed to contact daemon: %v (Is it running?)\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("🛑 Shutdown signal sent to Tardis daemon for project '%s'.\n", projectName)
	} else {
		fmt.Printf("❌ Shutdown request failed with status: %d\n", resp.StatusCode)
	}
}

func runPause(projectName string) {
	port, err := getDaemonPort(projectName)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return
	}
	url := fmt.Sprintf("http://localhost:%d/pause", port)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("❌ Failed to contact daemon: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("⏸️  Tardis daemon for project '%s' has been paused.\n", projectName)
	} else {
		fmt.Printf("❌ Pause request failed with status: %d\n", resp.StatusCode)
	}
}

func runResume(projectName string) {
	port, err := getDaemonPort(projectName)
	if err != nil {
		fmt.Printf("❌ %v\n", err)
		return
	}
	url := fmt.Sprintf("http://localhost:%d/resume", port)

	resp, err := http.Post(url, "application/json", nil)
	if err != nil {
		fmt.Printf("❌ Failed to contact daemon: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("▶️  Tardis daemon for project '%s' has been resumed.\n", projectName)
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

func arrowKeyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()

	nextKeys := key.NewBinding(
		key.WithKeys("down"),
		key.WithHelp("↓", "next"),
	)
	prevKeys := key.NewBinding(
		key.WithKeys("up"),
		key.WithHelp("↑", "prev"),
	)

	km.Input.Next = nextKeys
	km.Input.Prev = prevKeys
	km.Note.Next = nextKeys
	km.Note.Prev = prevKeys
	km.Confirm.Next = nextKeys
	km.Confirm.Prev = prevKeys

	km.Quit = key.NewBinding(
		key.WithKeys("ctrl+c", "esc"),
		key.WithHelp("esc", "quit"),
	)

	return km
}

type credSetupModel struct {
	form      *huh.Form
	quitting  bool
	showGuide bool
	guide     guideModel
}

func (m credSetupModel) Init() tea.Cmd {
	return tea.Batch(m.form.Init(), m.guide.Init())
}

func (m credSetupModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	if m.showGuide {
		switch msg := msg.(type) {
		case tea.KeyMsg:
			switch msg.String() {
			case "q", "esc":
				m.showGuide = false
				return m, nil
			case "ctrl+c":
				m.quitting = true
				return m, tea.Quit
			}
		case tea.WindowSizeMsg:
			updatedGuide, _ := m.guide.Update(msg)
			if g, ok := updatedGuide.(guideModel); ok {
				m.guide = g
			}
			// Update form with window size too just in case
			form, _ := m.form.Update(msg)
			if f, ok := form.(*huh.Form); ok {
				m.form = f
			}
			return m, nil
		}

		updatedGuide, cmd := m.guide.Update(msg)
		if g, ok := updatedGuide.(guideModel); ok {
			m.guide = g
		}
		return m, cmd
	}

	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "?":
			m.showGuide = true
			return m, nil
		case "ctrl+c", "esc":
			m.quitting = true
			return m, tea.Quit
		}
	case tea.WindowSizeMsg:
		// Send window size to guide even when hidden so it can initialize its viewport size
		updatedGuide, _ := m.guide.Update(msg)
		if g, ok := updatedGuide.(guideModel); ok {
			m.guide = g
		}
	}

	form, cmd := m.form.Update(msg)
	if f, ok := form.(*huh.Form); ok {
		m.form = f
	}

	if m.form.State == huh.StateCompleted {
		return m, tea.Quit
	}

	return m, cmd
}

func (m credSetupModel) View() string {
	if m.quitting {
		return ""
	}
	if m.showGuide {
		return m.guide.View()
	}
	footer := "\n  \033[2m[?] setup guide   [Esc] quit\033[0m"
	return m.form.View() + footer
}

func runCredSetup(credPath, rootDir, portStr *string, confirm *bool, gcpSetupURL string) bool {
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
				Value(credPath),
			huh.NewInput().
				Title("Port for this daemon (auto-detected):").
				Value(portStr).
				Validate(func(str string) error {
					p, err := strconv.Atoi(str)
					if err != nil || p <= 0 || p > 65535 {
						return errors.New("must be a valid port number (1-65535)")
					}
					return nil
				}),
			huh.NewInput().
				Title("Google Drive Root Directory Name:").
				Value(rootDir),
			huh.NewConfirm().
				Title("Ready to save configuration?").
				Value(confirm).
				Validate(func(v bool) error {
					if v {
						if *credPath == "" {
							return errors.New("credentials path cannot be empty")
						}
						if _, err := os.Stat(*credPath); os.IsNotExist(err) {
							return fmt.Errorf("credentials file not found: %s", *credPath)
						}
					}
					return nil
				}),
		),
	).WithKeyMap(arrowKeyMap())

	model := credSetupModel{
		form:  form2,
		guide: newGuideModel(assets.GCPGuideText),
	}
	p := tea.NewProgram(model, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return false
	}
	if m, ok := result.(credSetupModel); ok && m.quitting {
		return false
	}
	return true
}

func runRm(projectName string) {
	pidPath := getPIDFilePath(projectName)
	if data, err := os.ReadFile(pidPath); err == nil {
		var pid int
		if _, scanErr := fmt.Sscanf(string(data), "%d", &pid); scanErr == nil {
			if isProcessRunning(pid) {
				fmt.Printf("❌ Tardis daemon is currently running (PID: %d).\n", pid)
				fmt.Printf("💡 Please run 'tardis stop %s' first.\n", projectName)
				os.Exit(1)
			}
		}
	}

	projectDir := getProjectDir(projectName)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		fmt.Printf("❌ Project '%s' does not exist.\n", projectName)
		os.Exit(1)
	}

	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("⚠️ Are you sure you want to completely delete project '%s'?", projectName)).
				Description("This will remove all configurations, queued tasks (DB), and logs.").
				Value(&confirm),
		),
	)
	if err := form.Run(); err != nil || !confirm {
		fmt.Println("Aborted.")
		return
	}

	if err := os.RemoveAll(projectDir); err != nil {
		fmt.Printf("❌ Failed to remove project directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("🗑️ Project '%s' deleted successfully.\n", projectName)
}

func runLs() {
	showAll := false
	showLong := false
	for _, arg := range os.Args[2:] {
		if strings.Contains(arg, "a") {
			showAll = true
		}
		if strings.Contains(arg, "l") {
			showLong = true
		}
	}

	home, err := os.UserHomeDir()
	if err != nil {
		fmt.Printf("❌ Failed to get home dir: %v\n", err)
		return
	}
	projectsDir := filepath.Join(home, ".tardis", "projects")

	entries, err := os.ReadDir(projectsDir)
	if err != nil || len(entries) == 0 {
		fmt.Println("No Tardis projects found.")
		return
	}

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 3, ' ', 0)
	if showLong {
		fmt.Fprintln(w, "PROJECT\tSTATUS\tPID\tPORT\tPROVIDER\tROOT DIR")
	} else {
		fmt.Fprintln(w, "PROJECT\tSTATUS\tPID\tPORT")
	}

	count := 0
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		name := entry.Name()
		cfgPath := filepath.Join(projectsDir, name, "config.yml")
		cfg, err := config.Load(cfgPath)
		if err != nil {
			continue // skip invalid projects
		}

		pidPath := filepath.Join(projectsDir, name, "tardis.pid")
		status := "\033[90mStopped\033[0m"
		pidStr := "-"
		isRunning := false

		if data, err := os.ReadFile(pidPath); err == nil {
			var pid int
			if _, scanErr := fmt.Sscanf(string(data), "%d", &pid); scanErr == nil {
				if isProcessRunning(pid) {
					status = "\033[32mRunning\033[0m"
					pidStr = strconv.Itoa(pid)
					isRunning = true
				}
			}
		}

		if !isRunning && !showAll {
			continue
		}

		count++
		if showLong {
			provider := cfg.Storage.Provider
			rootDir := ""
			if provider == "google_drive" {
				rootDir = cfg.Storage.GoogleDrive.RootDir
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n", name, status, pidStr, cfg.Server.Port, provider, rootDir)
		} else {
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\n", name, status, pidStr, cfg.Server.Port)
		}
	}
	w.Flush()

	if count == 0 {
		if !showAll {
			fmt.Println("No running projects. Use 'tardis ls -a' to see all projects.")
		}
	}
}
