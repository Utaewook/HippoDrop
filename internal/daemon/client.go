package daemon

import (
	"fmt"
	"io"
	"net/http"
	"time"

	"hippodrop/internal/project"
)

func SendRequest(projectName string, method string, path string, body io.Reader) (*http.Response, error) {
	cfg, err := project.LoadConfig(projectName)
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
