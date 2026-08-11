package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strconv"
	"strings"
	"syscall"
	"text/tabwriter"

	"golang.org/x/oauth2/google"
	"google.golang.org/api/drive/v3"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"

	"hippodrop/internal/auth"
	"hippodrop/internal/browser"
	"hippodrop/internal/core"
	"hippodrop/internal/daemon"
	"hippodrop/internal/project"
	"hippodrop/internal/tui"
)

var Version = "dev"

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

func runInit(projectName string) {
	// 0. Check if project already exists
	cfgPath := project.ConfigPath(projectName)
	if _, err := os.Stat(cfgPath); err == nil {
		fmt.Printf("[Error] Project '%s' already exists.\n", projectName)
		fmt.Printf("[Hint] Remove project first: hippo rm %s\n", projectName)
		os.Exit(1)
	}

	var providerChoice string
	var credPath string
	var portStr = fmt.Sprintf("%d", project.NextAvailablePort())
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
				Value(&providerChoice),
		),
	).WithKeyMap(tui.ArrowKeyMap()).WithProgramOptions(tea.WithAltScreen())

	if err := form.Run(); err != nil {
		fmt.Println("Wizard aborted.")
		os.Exit(1)
	}

	if providerChoice != "Google Drive" {
		fmt.Printf("[Error] Provider '%s' is not supported.\n", providerChoice)
		os.Exit(1)
	}

	// Try to automatically open the Google Cloud Credentials page
	gcpSetupURL := "https://console.cloud.google.com/apis/credentials"
	_ = browser.Open(gcpSetupURL)

	if !tui.RunCredSetup(&credPath, &rootDir, &portStr, &confirm, gcpSetupURL) {
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

	// OAuth2 flow (Authorization Code + PKCE, manual redirect paste)
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
	gConfig.RedirectURL = auth.RedirectURL

	tok, err := auth.RunManualFlow(context.Background(), gConfig)
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

		logPath := filepath.Join(project.Dir(projectName), "daemon.log")
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

	configPath := project.ConfigPath(projectName)

	// Check if config exists
	if _, err := os.Stat(configPath); os.IsNotExist(err) {
		fmt.Printf("[Error] Config file not found at %s\n", configPath)
		fmt.Printf("[Hint] Run 'hippo init %s' to generate configuration.\n", projectName)
		os.Exit(1)
	}

	// Write PID file and check duplication
	if err := daemon.WritePIDFile(projectName); err != nil {
		fmt.Printf("[Error] %v\n", err)
		os.Exit(1)
	}
	defer daemon.RemovePIDFile(projectName)

	fmt.Printf("HippoDrop Daemon starting for project '%s'...\n", projectName)

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	cfg, err := project.LoadConfig(projectName)
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	if err := core.Run(ctx, cfg); err != nil {
		log.Fatalf("%v", err)
	}
}

func runStatus(projectName string) {
	resp, err := daemon.SendRequest(projectName, "GET", "/status", nil)
	if err != nil {
		pidPath := project.PIDFilePath(projectName)
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
	resp, err := daemon.SendRequest(projectName, "POST", "/stop", nil)
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
	resp, err := daemon.SendRequest(projectName, "POST", "/pause", nil)
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
	resp, err := daemon.SendRequest(projectName, "POST", "/resume", nil)
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

func runRm(projectName string) {
	pidPath := project.PIDFilePath(projectName)
	if data, err := os.ReadFile(pidPath); err == nil {
		var pid int
		if _, scanErr := fmt.Sscanf(string(data), "%d", &pid); scanErr == nil {
			if daemon.IsProcessRunning(pid) {
				fmt.Printf("[Error] HippoDrop daemon is currently running (PID: %d).\n", pid)
				fmt.Printf("[Hint] Run 'hippo stop %s' first.\n", projectName)
				os.Exit(1)
			}
		}
	}

	projectDir := project.Dir(projectName)
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
		cfg, err := project.LoadConfig(name)
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
				if daemon.IsProcessRunning(pid) {
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
			providerName := cfg.Storage.Provider
			rootDir := ""
			if providerName == "google_drive" {
				rootDir = cfg.Storage.GoogleDrive.RootDir
			}
			fmt.Fprintf(w, "%s\t%s\t%s\t%d\t%s\t%s\n", name, status, pidStr, cfg.Server.Port, providerName, rootDir)
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
