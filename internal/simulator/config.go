package simulator

import (
	"os"
	"gopkg.in/yaml.v3"
)

type Config struct {
	TargetURL    string          `yaml:"target_url"`
	PollInterval string          `yaml:"poll_interval"`
	WebhookPort  int             `yaml:"webhook_port"`
	WebhookURL   string          `yaml:"webhook_url"`
	Workspace    string          `yaml:"workspace"`
	Services     []ServiceConfig `yaml:"services"`
}

type ServiceConfig struct {
	Name      string `yaml:"name"`
	Interval  string `yaml:"interval"`
	SizeKB    int    `yaml:"size_kb"`
	RemoteDir string `yaml:"remote_dir"`
}

const defaultYAML = `target_url: "http://localhost:8080"
poll_interval: "10s"
webhook_port: 8081
webhook_url: "http://localhost:8081/webhook"
workspace: "./sim_data"

services:
  - name: "web-server"
    interval: "1s"
    size_kb: 10
    remote_dir: "logs/web"

  - name: "db-dump"
    interval: "10s"
    size_kb: 5000
    remote_dir: "dumps/db"

  - name: "analytics-batch"
    interval: "5s"
    size_kb: 1024
    remote_dir: "analytics/batch"
`

func LoadConfig(path string) (*Config, error) {
	var data []byte
	var err error

	// If the file path is the default one and it does not exist, use default embedded YAML.
	if path == "simulator.yml" {
		if _, statErr := os.Stat(path); os.IsNotExist(statErr) {
			data = []byte(defaultYAML)
		}
	}

	if data == nil {
		data, err = os.ReadFile(path)
		if err != nil {
			return nil, err
		}
	}

	var cfg Config
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, err
	}
	if cfg.Workspace == "" {
		cfg.Workspace = "./sim_data"
	}
	if cfg.PollInterval == "" {
		if cfg.WebhookPort > 0 {
			cfg.PollInterval = "10s"
		} else {
			cfg.PollInterval = "2s"
		}
	}
	return &cfg, nil
}
