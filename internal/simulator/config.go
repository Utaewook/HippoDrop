package simulator

import (
	"os"
	"gopkg.in/yaml.v3"
)

type Config struct {
	TargetURL    string          `yaml:"target_url"`
	PollInterval string          `yaml:"poll_interval"`
	Workspace    string          `yaml:"workspace"`
	Services     []ServiceConfig `yaml:"services"`
}

type ServiceConfig struct {
	Name      string `yaml:"name"`
	Interval  string `yaml:"interval"`
	SizeKB    int    `yaml:"size_kb"`
	RemoteDir string `yaml:"remote_dir"`
}

func LoadConfig(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Workspace == "" {
		cfg.Workspace = "./sim_data"
	}
	if cfg.PollInterval == "" {
		cfg.PollInterval = "2s"
	}
	return &cfg, nil
}
