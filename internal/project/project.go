package project

import (
	"fmt"
	"os"
	"path/filepath"

	"hippodrop/internal/config"
)

func Dir(projectName string) string {
	if home, err := os.UserHomeDir(); err == nil {
		return filepath.Join(home, ".hippodrop", "projects", projectName)
	}
	return fmt.Sprintf("./.hippodrop/projects/%s", projectName)
}

func ConfigPath(projectName string) string {
	return filepath.Join(Dir(projectName), "config.yml")
}

func LoadConfig(projectName string) (*config.Config, error) {
	cfgPath := ConfigPath(projectName)
	// Auto correct permissions to 0600 if file exists (Option X)
	if _, err := os.Stat(cfgPath); err == nil {
		_ = os.Chmod(cfgPath, 0600)
	}
	return config.Load(cfgPath)
}

func PIDFilePath(projectName string) string {
	return filepath.Join(Dir(projectName), "hippodrop.pid")
}

func NextAvailablePort() int {
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
