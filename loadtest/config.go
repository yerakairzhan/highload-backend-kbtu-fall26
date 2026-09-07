package main

import (
	"encoding/json"
	"os"
)

// Config holds the load-test parameters. Values come from a JSON file so they
// aren't hardcoded; command-line flags can still override any of them.
type Config struct {
	URL         string  `json:"url"`
	Requests    int     `json:"requests"`
	Concurrency int     `json:"concurrency"`
	TargetRPS   float64 `json:"target_rps"`
}

// defaultConfig is used when no config file is found.
func defaultConfig() Config {
	return Config{
		URL:         "http://localhost:8080",
		Requests:    200,
		Concurrency: 20,
		TargetRPS:   500,
	}
}

// loadConfig reads and parses the JSON config at path. A missing file is not an
// error — we fall back to defaults so the tool works out of the box.
func loadConfig(path string) (Config, error) {
	cfg := defaultConfig()
	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return cfg, nil
	}
	if err != nil {
		return cfg, err
	}
	if err := json.Unmarshal(data, &cfg); err != nil {
		return cfg, err
	}
	return cfg, nil
}
