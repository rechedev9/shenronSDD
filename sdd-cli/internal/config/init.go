package config

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var ErrAlreadyInitialized = errors.New("openspec/ already exists")

// InitResult holds what Init created.
type InitResult struct {
	ConfigPath string
	Dirs       []string
	Config     *Config
}

// Init bootstraps the openspec/ directory structure and writes config.yaml.
// If force is true, it overwrites an existing openspec/.
func Init(projectDir string, force bool) (*InitResult, error) {
	openspecDir := filepath.Join(projectDir, "openspec")
	configPath := filepath.Join(openspecDir, "config.yaml")

	// Check for existing openspec/.
	if _, err := os.Stat(openspecDir); err == nil && !force {
		return nil, fmt.Errorf("%w: use --force to reinitialize", ErrAlreadyInitialized)
	}

	// Detect stack.
	cfg, err := Detect(projectDir)
	if err != nil {
		return nil, fmt.Errorf("detect stack: %w", err)
	}

	// Create directory structure.
	dirs := []string{
		openspecDir,
		filepath.Join(openspecDir, "changes"),
		filepath.Join(openspecDir, "changes", "archive"),
	}
	for _, d := range dirs {
		if err := os.MkdirAll(d, 0o755); err != nil {
			return nil, fmt.Errorf("create directory %s: %w", d, err)
		}
	}

	// Write config.
	if err := Save(cfg, configPath); err != nil {
		return nil, fmt.Errorf("write config: %w", err)
	}

	return &InitResult{
		ConfigPath: configPath,
		Dirs:       dirs,
		Config:     cfg,
	}, nil
}
