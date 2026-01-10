package config

import (
	"encoding/json"
	"fmt"
	"os"
)

// Config holds application configuration.
type Config struct {
	// Add fields as needed
}

// Load reads configuration from a file.
func Load(path string) (*Config, error) {
	if path == "" {
		return &Config{}, nil
	}

	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading config file: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parsing config file: %w", err)
	}

	return &cfg, nil
}
