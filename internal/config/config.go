package config

import (
	"fmt"
	"os"

	"gopkg.in/yaml.v3"
)

// Config represents the root configuration structure for HippoDrop.
type Config struct {
	Server  ServerConfig  `yaml:"server"`
	Storage StorageConfig `yaml:"storage"`
	Workers WorkersConfig `yaml:"workers"`
}

type ServerConfig struct {
	Port    int    `yaml:"port"`
	DataDir string `yaml:"data_dir"`
	APIKey  string `yaml:"api_key"`
}

type StorageConfig struct {
	Provider    string            `yaml:"provider"`
	GoogleDrive GoogleDriveConfig `yaml:"google_drive"`
}

type GoogleDriveConfig struct {
	CredentialsPath   string `yaml:"credentials_path"`
	TokenPath         string `yaml:"token_path"`
	RateLimitPerSec   int    `yaml:"rate_limit_per_second"`
	RetryMaxAttempts  int    `yaml:"retry_max_attempts"`
	RootDir           string `yaml:"root_dir"`
}

type WorkersConfig struct {
	PoolSize    int `yaml:"pool_size"`
	ChunkSizeMB int `yaml:"chunk_size_mb"`
}

// Load reads and parses the YAML configuration file from the given path.
func Load(path string) (*Config, error) {
	file, err := os.Open(path)
	if err != nil {
		return nil, fmt.Errorf("failed to open config file: %w", err)
	}
	defer file.Close()

	var cfg Config
	decoder := yaml.NewDecoder(file)
	if err := decoder.Decode(&cfg); err != nil {
		return nil, fmt.Errorf("failed to decode config file: %w", err)
	}

	return &cfg, nil
}
