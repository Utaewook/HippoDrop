package config

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoad_ValidConfig(t *testing.T) {
	// Create a temporary directory for test configuration file
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "tardis_test.yml")

	// Write a valid YAML config content
	validYAML := `
server:
  port: 8080
  data_dir: "/tmp/tardis"
storage:
  provider: "google_drive"
  google_drive:
    credentials_path: "credentials.json"
    rate_limit_per_second: 10
    retry_max_attempts: 5
    root_dir: "root"
workers:
  pool_size: 4
  chunk_size_mb: 10
`
	err := os.WriteFile(configPath, []byte(validYAML), 0644)
	if err != nil {
		t.Fatalf("failed to create temporary config file: %v", err)
	}

	// Load the config
	cfg, err := Load(configPath)
	if err != nil {
		t.Fatalf("Load returned unexpected error: %v", err)
	}

	// Verify the values
	if cfg.Server.Port != 8080 {
		t.Errorf("expected port 8080, got %d", cfg.Server.Port)
	}
	if cfg.Server.DataDir != "/tmp/tardis" {
		t.Errorf("expected data_dir /tmp/tardis, got %s", cfg.Server.DataDir)
	}
	if cfg.Storage.Provider != "google_drive" {
		t.Errorf("expected provider google_drive, got %s", cfg.Storage.Provider)
	}
	if cfg.Storage.GoogleDrive.CredentialsPath != "credentials.json" {
		t.Errorf("expected credentials_path credentials.json, got %s", cfg.Storage.GoogleDrive.CredentialsPath)
	}
	if cfg.Storage.GoogleDrive.RateLimitPerSec != 10 {
		t.Errorf("expected rate_limit_per_second 10, got %d", cfg.Storage.GoogleDrive.RateLimitPerSec)
	}
	if cfg.Storage.GoogleDrive.RetryMaxAttempts != 5 {
		t.Errorf("expected retry_max_attempts 5, got %d", cfg.Storage.GoogleDrive.RetryMaxAttempts)
	}
	if cfg.Storage.GoogleDrive.RootDir != "root" {
		t.Errorf("expected root_dir root, got %s", cfg.Storage.GoogleDrive.RootDir)
	}
	if cfg.Workers.PoolSize != 4 {
		t.Errorf("expected pool_size 4, got %d", cfg.Workers.PoolSize)
	}
	if cfg.Workers.ChunkSizeMB != 10 {
		t.Errorf("expected chunk_size_mb 10, got %d", cfg.Workers.ChunkSizeMB)
	}
}

func TestLoad_NonExistentFile(t *testing.T) {
	_, err := Load("non_existent_file.yml")
	if err == nil {
		t.Fatal("expected an error loading non-existent file, got nil")
	}
}

func TestLoad_InvalidYAML(t *testing.T) {
	tempDir := t.TempDir()
	configPath := filepath.Join(tempDir, "invalid_test.yml")

	// Write invalid YAML format
	invalidYAML := `
server:
  port: [invalid_array_port
`
	err := os.WriteFile(configPath, []byte(invalidYAML), 0644)
	if err != nil {
		t.Fatalf("failed to create temporary config file: %v", err)
	}

	_, err = Load(configPath)
	if err == nil {
		t.Fatal("expected an error decoding invalid YAML, got nil")
	}
}
