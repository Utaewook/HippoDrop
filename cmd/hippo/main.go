package main

import (
	"context"
	"crypto/rand"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"net/http"
	"net/url"
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

	"golang.org/x/oauth2"
	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/bubbles/viewport"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"hippodrop/internal/api"
	"hippodrop/internal/assets"
	"hippodrop/internal/config"
	"hippodrop/internal/queue"
	"hippodrop/internal/storage"
	"hippodrop/internal/worker"
)

var Version = "dev"

func getProjectDir(projectName string) string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".hippodrop", "projects", projectName)
	}
	return fmt.Sprintf("./.hippodrop/projects/%s", projectName)
}

func getProjectConfigPath(projectName string) string {
	return filepath.Join(getProjectDir(projectName), "config.yml")
}

func loadProjectConfig(projectName string) (*config.Config, error) {
	cfgPath := getProjectConfigPath(projectName)
	// Auto correct permissions to 0600 if file exists (Option X)
	if _, err := os.Stat(cfgPath); err == nil {
		_ = os.Chmod(cfgPath, 0600)
	}
	return config.Load(cfgPath)
}

func getPIDFilePath(projectName string) string {
	return filepath.Join(getProjectDir(projectName), "hippodrop.pid")
}

func main() {
	if len(os.Args) < 2 {
		printUsage()
		os.Exit(1)
	}

	command := os.Args[1]

	switch command {
	case "--version", "-v":
		fmt.Printf("HippoDrop %s\n", Version)
		return
	case "--help", "-h":
		printUsage()
		return
	case "ls":
		runLs()
		return
	}

	if len(os.Args) < 3 {
		fmt.Printf("[Error] Missing project name.\n\n")
		printUsage()
		os.Exit(1)
	}

	projectName := os.Args[2]

	if strings.TrimSpace(projectName) == "" {
		fmt.Println("[Error] Project name cannot be empty.")
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
		fmt.Printf("[Error] Unknown command: %s\n\n", command)
		printUsage()
		os.Exit(1)
	}
}

func printUsage() {
	fmt.Println("HippoDrop Cloud Storage Proxy Daemon")
	fmt.Println("\nUsage:")
	fmt.Println("  hippo init <project>         Launch setup wizard for a new project")
	fmt.Println("  hippo start <project> [-d]   Start the daemon (use -d for background)")
	fmt.Println("  hippo ls [-a] [-l]           List daemons (-a: all, -l: details)")
	fmt.Println("  hippo status <project>       Check the daemon running status")
	fmt.Println("  hippo stop <project>         Stop the running daemon gracefully")
	fmt.Println("  hippo pause <project>        Pause dispatching new tasks")
	fmt.Println("  hippo resume <project>       Resume dispatching tasks")
	fmt.Println("  hippo rm <project>           Remove a stopped project completely")
	fmt.Println("  hippo --version              Show version")
	fmt.Println("  hippo --help                 Show help")
}

func getNextAvailablePort() int {
	home, err := os.UserHomeDir()
	if err != nil {
		return 8080
	}
	projectsDir := filepath.Join(home, ".hippodrop", "projects")
	
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
	// 0. Check if project already exists
	cfgPath := getProjectConfigPath(projectName)
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("[Error] Project '%s' already exists.\n", projectName)
		fmt.Printf("[Hint] Remove project first: hippo rm %s\n", projectName)
		os.Exit(1)
	}

	var provider string
	var credPath string
	var portStr = fmt.Sprintf("%d", getNextAvailablePort())
	var rootDir string = projectName
	var confirm bool

	form := huh.NewForm(
		huh.NewGroup(
			huh.NewNote().
				Title("HippoDrop Setup Wizard").
				Description(fmt.Sprintf("Configure project '%s'.\nPress Enter to continue.", projectName)),
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
		fmt.Printf("[Error] Provider '%s' is not supported.\n", provider)
		os.Exit(1)
	}

	// Try to automatically open the Google Cloud Credentials page
	gcpSetupURL := "https://console.cloud.google.com/apis/credentials"
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
		fmt.Printf("[Error] Invalid port number: %s\n", portStr)
		os.Exit(1)
	}

	// Save Configuration
	cfgDir := filepath.Dir(cfgPath)

	if err := os.MkdirAll(cfgDir, 0755); err != nil {
		fmt.Printf("\n[Error] Failed to create config directory at %s: %v\n", cfgDir, err)
		os.Exit(1)
	}

	dataDir := filepath.Join(cfgDir, "data")
	tokenPath := filepath.Join(cfgDir, "token.json")

	// OAuth2 flow
	b, err := os.ReadFile(credPath)
	if err != nil {
		fmt.Printf("[Error] Unable to read client secret file: %v\n", err)
		os.Exit(1)
	}

	gConfig, err := google.ConfigFromJSON(b, drive.DriveFileScope, drive.DriveScope)
	if err != nil {
		fmt.Printf("[Error] Unable to parse client secret file: %v\n", err)
		os.Exit(1)
	}

	deviceRes, err := requestDeviceCode(gConfig.ClientID, strings.Join(gConfig.Scopes, " "))
	if err != nil {
		fmt.Printf("[Error] Unable to request device code from Google: %v\n", err)
		os.Exit(1)
	}

	tok, err := runOAuthPolling(gConfig.ClientID, gConfig.ClientSecret, deviceRes)
	if err != nil {
		fmt.Printf("\n[Error] Google OAuth authentication failed: %v\n", err)
		os.Exit(1)
	}

	tokFile, err := os.OpenFile(tokenPath, os.O_RDWR|os.O_CREATE|os.O_TRUNC, 0600)
	if err != nil {
		fmt.Printf("[Error] Unable to save oauth token: %v\n", err)
		os.Exit(1)
	}
	json.NewEncoder(tokFile).Encode(tok)
	tokFile.Close()

	configTemplate := fmt.Sprintf(`server:
  port: %d
  data_dir: "%s"

storage:
  provider: "google_drive"
  google_drive:
    credentials_path: "%s"
    token_path: "%s"
    rate_limit_per_second: 10
    retry_max_attempts: 5
    root_dir: "%s"

workers:
  pool_size: 4
  chunk_size_mb: 10
`, port, dataDir, credPath, tokenPath, rootDir)

	if err := os.WriteFile(cfgPath, []byte(configTemplate), 0600); err != nil {
		fmt.Printf("\n[Error] Failed to save config file: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("\nProject '%s' configured.\n", projectName)
	fmt.Printf("Configuration saved to: %s\n", cfgPath)
	fmt.Println("\nYou can now start the daemon by running:")
	fmt.Printf("Run:  hippo start %s\n", projectName)
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
			fmt.Printf("[Error] Failed to create log directory: %v\n", err)
			os.Exit(1)
		}
		
		logFile, err := os.OpenFile(logPath, os.O_CREATE|os.O_WRONLY|os.O_APPEND, 0644)
		if err != nil {
			fmt.Printf("[Error] Failed to open daemon.log: %v\n", err)
			os.Exit(1)
		}
		cmd.Stdout = logFile
		cmd.Stderr = logFile

		if err := cmd.Start(); err != nil {
			fmt.Printf("[Error] Failed to start daemon in background: %v\n", err)
			os.Exit(1)
		}
		
		fmt.Printf("HippoDrop daemon started in background (PID: %d)\n", cmd.Process.Pid)
		fmt.Printf("Logs: %s\n", logPath)
		return
	}

	configPath := getProjectConfigPath(projectName)

	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("[Error] Config file not found at %s\n", configPath)
		fmt.Printf("[Hint] Run 'hippo init %s' to generate configuration.\n", projectName)
		os.Exit(1)
	}
	
	// Write PID file and check duplication
	if err := writePIDFile(projectName); err != nil {
		fmt.Printf("[Error] %v\n", err)
		os.Exit(1)
	}
	defer removePIDFile(projectName)
	
	fmt.Printf("HippoDrop Daemon starting for project '%s'...\n", projectName)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	// 2. Load Configuration
	cfg, err := loadProjectConfig(projectName)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	// 3. Initialize Database (Queue & Cache)
	db, err := queue.Open(cfg.Server.DataDir)
	if err != nil {
		log.Fatalf("Failed to initialize database: %v", err)
	}
	defer db.Close()
	log.Printf("SQLite DB initialized at %s/hippodrop.db", cfg.Server.DataDir)

	// 4. Initialize Storage Provider
	var provider storage.Provider
	if cfg.Storage.Provider == "google_drive" {
		provider, err = storage.NewGoogleDriveAdapter(context.Background(), cfg.Storage.GoogleDrive.CredentialsPath, cfg.Storage.GoogleDrive.TokenPath, cfg.Storage.GoogleDrive.RootDir, db)
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

	// 7.5. Start Webhook Sender
	webhookSender := worker.NewWebhookSender(db, cfg.Server.APIKey)
	go webhookSender.Start(ctx)

	// 8. Setup HTTP API
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

	// 9. Start HTTP Server in background
	go func() {
		log.Printf("Server is running on port %d", cfg.Server.Port)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("HTTP server failed: %v", err)
		}
	}()

	// 10. Wait for SIGINT/SIGTERM
	<-ctx.Done()
	log.Println("\nReceived shutdown signal. Stopping daemon...")

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

	log.Println("HippoDrop daemon stopped.")
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
				return fmt.Errorf("hippodrop daemon is already running for project '%s' (PID: %d)", projectName, pid)
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

func sendDaemonRequest(projectName string, method string, path string, body io.Reader) (*http.Response, error) {
	cfg, err := loadProjectConfig(projectName)
	if err != nil {
		return nil, fmt.Errorf("could not load config for project %s: %w", projectName, err)
	}

	port := cfg.Server.Port
	if port == 0 {
		return nil, fmt.Errorf("invalid port in config for project %s", projectName)
	}

	url := fmt.Sprintf("http://localhost:%d%s", port, path)
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, fmt.Errorf("failed to create HTTP request: %w", err)
	}

	if method == "POST" {
		req.Header.Set("Content-Type", "application/json")
	}

	if cfg.Server.APIKey != "" {
		req.Header.Set("Authorization", fmt.Sprintf("Bearer %s", cfg.Server.APIKey))
	}

	client := &http.Client{Timeout: 5 * time.Second}
	return client.Do(req)
}

func runStatus(projectName string) {
	resp, err := sendDaemonRequest(projectName, "GET", "/status", nil)
	if err != nil {
		pidPath := getPIDFilePath(projectName)
		if _, statErr := os.Stat(pidPath); statErr == nil {
			fmt.Println("[Warning] PID file exists but daemon is not responding. (Status: Stale)")
		} else {
			fmt.Printf("[Info] HippoDrop daemon for project '%s' is not running.\n", projectName)
		}
		return
	}
	defer resp.Body.Close()

	var res map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		fmt.Printf("[Error] Failed to parse status response: %v\n", err)
		return
	}

	fmt.Printf("● Project '%s' Status: %v (PID: %v)\n", projectName, res["status"], res["pid"])
}

func runStop(projectName string) {
	resp, err := sendDaemonRequest(projectName, "POST", "/stop", nil)
	if err != nil {
		fmt.Printf("[Error] Failed to contact daemon: %v (Is it running?)\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("Shutdown signal sent to HippoDrop daemon for project '%s'.\n", projectName)
	} else {
		fmt.Printf("[Error] Shutdown request failed with status: %d\n", resp.StatusCode)
	}
}

func runPause(projectName string) {
	resp, err := sendDaemonRequest(projectName, "POST", "/pause", nil)
	if err != nil {
		fmt.Printf("[Error] Failed to contact daemon: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("[Paused] HippoDrop daemon for project '%s' has been paused.\n", projectName)
	} else {
		fmt.Printf("[Error] Pause request failed with status: %d\n", resp.StatusCode)
	}
}

func runResume(projectName string) {
	resp, err := sendDaemonRequest(projectName, "POST", "/resume", nil)
	if err != nil {
		fmt.Printf("[Error] Failed to contact daemon: %v\n", err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusOK {
		fmt.Printf("[Resumed] HippoDrop daemon for project '%s' has been resumed.\n", projectName)
	} else {
		fmt.Printf("[Error] Resume request failed with status: %d\n", resp.StatusCode)
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
	header := "Google Drive GCP Setup Guide (Press 'q' or 'Esc' to exit)\n-----------------------------------------------------------"
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
				Title("Google Drive Setup").
				Description(fmt.Sprintf(
					"HippoDrop connects to your Google Drive via OAuth 2.0.\n\n"+
						"[Warning] NEVER USED GOOGLE CLOUD BEFORE?\n"+
						"Press '?' on your keyboard to open the beginner's step-by-step guide.\n\n"+
						"Quick Summary (if you know what you're doing):\n"+
						"1. Go to: %s\n"+
						"2. Set up OAuth Consent Screen (Add your Gmail to 'Test users'!).\n"+
						"3. Create an OAuth Client ID (Type: 'TVs and Limited Input Devices').\n"+
						"4. Download the JSON file and enter its absolute path below.",
					gcpSetupURL,
				)),
			huh.NewInput().
				Title("Absolute path to client_secret.json:").
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
				fmt.Printf("[Error] HippoDrop daemon is currently running (PID: %d).\n", pid)
				fmt.Printf("[Hint] Run 'hippo stop %s' first.\n", projectName)
				os.Exit(1)
			}
		}
	}

	projectDir := getProjectDir(projectName)
	if _, err := os.Stat(projectDir); os.IsNotExist(err) {
		fmt.Printf("[Error] Project '%s' does not exist.\n", projectName)
		os.Exit(1)
	}

	var confirm bool
	form := huh.NewForm(
		huh.NewGroup(
			huh.NewConfirm().
				Title(fmt.Sprintf("[Warning] Are you sure you want to completely delete project '%s'?", projectName)).
				Description("This will remove all configurations, queued tasks (DB), and logs.").
				Value(&confirm),
		),
	)
	if err := form.Run(); err != nil || !confirm {
		fmt.Println("Aborted.")
		return
	}

	if err := os.RemoveAll(projectDir); err != nil {
		fmt.Printf("[Error] Failed to remove project directory: %v\n", err)
		os.Exit(1)
	}

	fmt.Printf("Project '%s' deleted successfully.\n", projectName)
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
		fmt.Printf("[Error] Failed to get home dir: %v\n", err)
		return
	}
	projectsDir := filepath.Join(home, ".hippodrop", "projects")

	entries, err := os.ReadDir(projectsDir)
	if err != nil || len(entries) == 0 {
		fmt.Println("No HippoDrop projects found.")
		return
	}

	var sb strings.Builder
	w := tabwriter.NewWriter(&sb, 0, 0, 3, ' ', 0)
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
		cfg, err := loadProjectConfig(name)
		if err != nil {
			continue // skip invalid projects
		}

		pidPath := filepath.Join(projectsDir, name, "hippodrop.pid")
		status := "Stopped"
		pidStr := "-"
		isRunning := false

		if data, err := os.ReadFile(pidPath); err == nil {
			var pid int
			if _, scanErr := fmt.Sscanf(string(data), "%d", &pid); scanErr == nil {
				if isProcessRunning(pid) {
					status = "Running"
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

	out := sb.String()
	out = strings.ReplaceAll(out, "Running", "\033[32mRunning\033[0m")
	out = strings.ReplaceAll(out, "Stopped", "\033[90mStopped\033[0m")
	fmt.Print(out)
}

func generateCodeVerifier() string {
	b := make([]byte, 32)
	_, _ = rand.Read(b)
	return base64.RawURLEncoding.EncodeToString(b)
}

type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURL         string `json:"verification_url"`
	VerificationURLComplete string `json:"verification_url_complete,omitempty"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

func requestDeviceCode(clientID, scope string) (*DeviceCodeResponse, error) {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("scope", scope)

	resp, err := http.PostForm("https://oauth2.googleapis.com/device/code", v)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("device/code request failed: status %d, body %s", resp.StatusCode, string(body))
	}

	var res DeviceCodeResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, err
	}
	return &res, nil
}

func requestToken(clientID, clientSecret, deviceCode string) (*oauth2.Token, error) {
	v := url.Values{}
	v.Set("client_id", clientID)
	v.Set("client_secret", clientSecret)
	v.Set("device_code", deviceCode)
	v.Set("grant_type", "urn:ietf:params:oauth:grant-type:device_code")

	resp, err := http.PostForm("https://oauth2.googleapis.com/token", v)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}

	if resp.StatusCode != http.StatusOK {
		var errRes struct {
			Error            string `json:"error"`
			ErrorDescription string `json:"error_description"`
		}
		if err := json.Unmarshal(body, &errRes); err == nil {
			return nil, fmt.Errorf("oauth_error: %s", errRes.Error)
		}
		return nil, fmt.Errorf("status code %d", resp.StatusCode)
	}

	var tokRes struct {
		AccessToken  string `json:"access_token"`
		TokenType    string `json:"token_type"`
		RefreshToken string `json:"refresh_token"`
		ExpiresIn    int64  `json:"expires_in"`
	}
	if err := json.Unmarshal(body, &tokRes); err != nil {
		return nil, err
	}

	tok := &oauth2.Token{
		AccessToken:  tokRes.AccessToken,
		TokenType:    tokRes.TokenType,
		RefreshToken: tokRes.RefreshToken,
	}
	if tokRes.ExpiresIn > 0 {
		tok.Expiry = time.Now().Add(time.Duration(tokRes.ExpiresIn) * time.Second)
	}
	return tok, nil
}

func isPendingError(err error) bool {
	return strings.Contains(err.Error(), "oauth_error: authorization_pending")
}

func isSlowDownError(err error) bool {
	return strings.Contains(err.Error(), "oauth_error: slow_down")
}

type oauthPollingModel struct {
	clientID     string
	clientSecret string
	deviceCode   string
	userCode     string
	authURL      string
	interval     time.Duration
	expiresAt    time.Time
	dots         string
	token        *oauth2.Token
	err          error
	quitting     bool
}

type animateMsg struct{}
type pollAttemptMsg struct{}
type tokenSuccessMsg struct {
	token *oauth2.Token
}
type tokenErrorMsg struct {
	err error
}

func tickAnimate() tea.Cmd {
	return tea.Tick(500*time.Millisecond, func(t time.Time) tea.Msg {
		return animateMsg{}
	})
}

func (m oauthPollingModel) Init() tea.Cmd {
	_ = openBrowser(m.authURL)
	return tea.Batch(
		tickAnimate(),
		tea.Tick(m.interval, func(t time.Time) tea.Msg {
			return pollAttemptMsg{}
		}),
	)
}

func (m oauthPollingModel) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		switch msg.String() {
		case "esc", "ctrl+c":
			m.quitting = true
			m.err = errors.New("authentication cancelled by user")
			return m, tea.Quit
		}

	case animateMsg:
		if m.quitting {
			return m, nil
		}
		if m.dots == "..." {
			m.dots = "."
		} else {
			m.dots += "."
		}
		return m, tickAnimate()

	case pollAttemptMsg:
		if m.quitting {
			return m, nil
		}
		if time.Now().After(m.expiresAt) {
			m.err = errors.New("authorization code expired")
			m.quitting = true
			return m, tea.Quit
		}

		return m, func() tea.Msg {
			tok, err := requestToken(m.clientID, m.clientSecret, m.deviceCode)
			if err != nil {
				return tokenErrorMsg{err: err}
			}
			return tokenSuccessMsg{token: tok}
		}

	case tokenSuccessMsg:
		m.token = msg.token
		m.quitting = true
		return m, tea.Quit

	case tokenErrorMsg:
		if m.quitting {
			return m, nil
		}
		if isPendingError(msg.err) {
			return m, tea.Tick(m.interval, func(t time.Time) tea.Msg {
				return pollAttemptMsg{}
			})
		}
		if isSlowDownError(msg.err) {
			m.interval += 5 * time.Second
			return m, tea.Tick(m.interval, func(t time.Time) tea.Msg {
				return pollAttemptMsg{}
			})
		}
		m.err = msg.err
		m.quitting = true
		return m, tea.Quit
	}
	return m, nil
}

func (m oauthPollingModel) View() string {
	if m.quitting {
		return ""
	}

	var sb strings.Builder
	sb.WriteString("\n")
	sb.WriteString("  \033[1;36mHippoDrop Google Drive Authorization (Device Flow)\033[0m\n")
	sb.WriteString("  \033[36m────────────────────────────────────────────────────────────\033[0m\n\n")
	sb.WriteString("  \033[1m1. 다음 URL을 웹 브라우저에서 열어주세요:\033[0m\n")
	sb.WriteString(fmt.Sprintf("     👉 \033[32;4m%s\033[0m\n\n", m.authURL))
	sb.WriteString("  \033[1m2. 브라우저 인증 화면에 아래 활성화 코드를 입력해주세요:\033[0m\n")
	sb.WriteString(fmt.Sprintf("     👉 \033[1;33m%s\033[0m\n\n", m.userCode))
	sb.WriteString("  \033[36m────────────────────────────────────────────────────────────\033[0m\n\n")
	sb.WriteString(fmt.Sprintf("  ⌛ \033[36m인증 대기 중%s\033[0m\n", m.dots))
	sb.WriteString("  \033[2m(취소하려면 Esc 또는 Ctrl+C를 누르세요)\033[0m\n")

	return sb.String()
}

func runOAuthPolling(clientID, clientSecret string, deviceRes *DeviceCodeResponse) (*oauth2.Token, error) {
	authURL := deviceRes.VerificationURLComplete
	if authURL == "" {
		authURL = deviceRes.VerificationURL
	}

	model := oauthPollingModel{
		clientID:     clientID,
		clientSecret: clientSecret,
		deviceCode:   deviceRes.DeviceCode,
		userCode:     deviceRes.UserCode,
		authURL:      authURL,
		interval:     time.Duration(deviceRes.Interval) * time.Second,
		expiresAt:    time.Now().Add(time.Duration(deviceRes.ExpiresIn) * time.Second),
		dots:         ".",
	}
	if model.interval == 0 {
		model.interval = 5 * time.Second
	}

	p := tea.NewProgram(model, tea.WithAltScreen())
	result, err := p.Run()
	if err != nil {
		return nil, err
	}

	m, ok := result.(oauthPollingModel)
	if !ok {
		return nil, errors.New("invalid TUI model type")
	}

	if m.err != nil {
		return nil, m.err
	}

	if m.token == nil {
		return nil, errors.New("no token received")
	}

	return m.token, nil
}
