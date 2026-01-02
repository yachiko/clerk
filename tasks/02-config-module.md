# Task 02: Configuration Module

## Objective
Implement the configuration system that manages user preferences and defaults stored in `$HOME/.clerk/config.json`.

## Prerequisites
- Task 01 completed (project setup)

## Deliverables

### 1. Create Config Types

Create file `internal/config/types.go`:

```go
package config

import "time"

// Config represents the application configuration
type Config struct {
	Region           string        `json:"region"`
	Profile          string        `json:"profile"`
	CachePath        string        `json:"cache_path"`
	CacheTTL         time.Duration `json:"cache_ttl"`
	ClipboardTimeout time.Duration `json:"clipboard_timeout"`
	DefaultType      string        `json:"default_type"`
	DefaultSort      string        `json:"default_sort"`
	ParallelFetches  int           `json:"parallel_fetches"`
}

// DefaultConfig returns a Config with default values
func DefaultConfig() *Config {
	return &Config{
		Region:           "us-east-1",
		Profile:          "default",
		CachePath:        "", // Will be set to $HOME/.clerk/cache.json
		CacheTTL:         3 * time.Hour,
		ClipboardTimeout: 60 * time.Second,
		DefaultType:      "SecureString",
		DefaultSort:      "name",
		ParallelFetches:  10,
	}
}

// ValidTypes returns valid parameter types
func ValidTypes() []string {
	return []string{"String", "StringList", "SecureString"}
}

// ValidSortOptions returns valid sort options
func ValidSortOptions() []string {
	return []string{"name", "created", "modified", "n", "c", "m"}
}
```

### 2. Create Config Manager

Create file `internal/config/config.go`:

```go
package config

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"
)

const (
	configDirName  = ".clerk"
	configFileName = "config.json"
)

// Manager handles configuration operations
type Manager struct {
	configPath string
	config     *Config
}

// NewManager creates a new configuration manager
func NewManager() (*Manager, error) {
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return nil, fmt.Errorf("failed to get home directory: %w", err)
	}

	configDir := filepath.Join(homeDir, configDirName)
	configPath := filepath.Join(configDir, configFileName)

	m := &Manager{
		configPath: configPath,
		config:     DefaultConfig(),
	}

	// Set default cache path
	m.config.CachePath = filepath.Join(configDir, "cache.json")

	// Load existing config if it exists
	if err := m.load(); err != nil && !os.IsNotExist(err) {
		return nil, err
	}

	return m, nil
}

// load reads configuration from disk
func (m *Manager) load() error {
	data, err := os.ReadFile(m.configPath)
	if err != nil {
		return err
	}

	return json.Unmarshal(data, m.config)
}

// Save writes configuration to disk
func (m *Manager) Save() error {
	dir := filepath.Dir(m.configPath)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}

	data, err := json.MarshalIndent(m.config, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal config: %w", err)
	}

	if err := os.WriteFile(m.configPath, data, 0600); err != nil {
		return fmt.Errorf("failed to write config: %w", err)
	}

	return nil
}

// Get returns the current configuration
func (m *Manager) Get() *Config {
	return m.config
}

// GetValue returns a configuration value by key
func (m *Manager) GetValue(key string) (string, error) {
	switch strings.ToLower(key) {
	case "region":
		return m.config.Region, nil
	case "profile":
		return m.config.Profile, nil
	case "cache_path", "cache.path":
		return m.config.CachePath, nil
	case "cache_ttl", "cache.ttl":
		return m.config.CacheTTL.String(), nil
	case "clipboard_timeout", "clipboard.timeout":
		return m.config.ClipboardTimeout.String(), nil
	case "default_type", "default.type":
		return m.config.DefaultType, nil
	case "default_sort", "default.sort":
		return m.config.DefaultSort, nil
	case "parallel_fetches", "parallel.fetches":
		return strconv.Itoa(m.config.ParallelFetches), nil
	default:
		return "", fmt.Errorf("unknown configuration key: %s", key)
	}
}

// SetValue sets a configuration value by key
func (m *Manager) SetValue(key, value string) error {
	switch strings.ToLower(key) {
	case "region":
		m.config.Region = value
	case "profile":
		m.config.Profile = value
	case "cache_path", "cache.path":
		m.config.CachePath = value
	case "cache_ttl", "cache.ttl":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration format: %w", err)
		}
		m.config.CacheTTL = d
	case "clipboard_timeout", "clipboard.timeout":
		d, err := time.ParseDuration(value)
		if err != nil {
			return fmt.Errorf("invalid duration format: %w", err)
		}
		m.config.ClipboardTimeout = d
	case "default_type", "default.type":
		if !isValidType(value) {
			return fmt.Errorf("invalid type: %s (valid: %v)", value, ValidTypes())
		}
		m.config.DefaultType = value
	case "default_sort", "default.sort":
		if !isValidSort(value) {
			return fmt.Errorf("invalid sort option: %s (valid: %v)", value, ValidSortOptions())
		}
		m.config.DefaultSort = value
	case "parallel_fetches", "parallel.fetches":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 50 {
			return fmt.Errorf("parallel_fetches must be between 1 and 50")
		}
		m.config.ParallelFetches = n
	default:
		return fmt.Errorf("unknown configuration key: %s", key)
	}
	return nil
}

// ListKeys returns all available configuration keys
func (m *Manager) ListKeys() []string {
	return []string{
		"region",
		"profile",
		"cache_path",
		"cache_ttl",
		"clipboard_timeout",
		"default_type",
		"default_sort",
		"parallel_fetches",
	}
}

func isValidType(t string) bool {
	for _, valid := range ValidTypes() {
		if strings.EqualFold(t, valid) {
			return true
		}
	}
	return false
}

func isValidSort(s string) bool {
	for _, valid := range ValidSortOptions() {
		if strings.EqualFold(s, valid) {
			return true
		}
	}
	return false
}
```

### 3. Create Config CLI Command

Create file `internal/cli/config.go`:

```go
package cli

import (
	"fmt"

	"github.com/fatih/color"
	"github.com/spf13/cobra"
	"github.com/yachiko/clerk/internal/config"
)

func init() {
	rootCmd.AddCommand(configCmd)
	configCmd.AddCommand(configGetCmd)
	configCmd.AddCommand(configSetCmd)
	configCmd.AddCommand(configListCmd)
}

var configCmd = &cobra.Command{
	Use:   "config",
	Short: "Manage clerk configuration",
	Long:  `Get, set, or list configuration options for the clerk CLI tool.`,
}

var configGetCmd = &cobra.Command{
	Use:   "get <key>",
	Short: "Get a configuration value",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := config.NewManager()
		if err != nil {
			return err
		}

		value, err := mgr.GetValue(args[0])
		if err != nil {
			return err
		}

		fmt.Println(value)
		return nil
	},
}

var configSetCmd = &cobra.Command{
	Use:   "set <key> <value>",
	Short: "Set a configuration value",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := config.NewManager()
		if err != nil {
			return err
		}

		if err := mgr.SetValue(args[0], args[1]); err != nil {
			return err
		}

		if err := mgr.Save(); err != nil {
			return err
		}

		color.Green("Configuration updated: %s = %s", args[0], args[1])
		return nil
	},
}

var configListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configuration values",
	RunE: func(cmd *cobra.Command, args []string) error {
		mgr, err := config.NewManager()
		if err != nil {
			return err
		}

		bold := color.New(color.Bold)
		for _, key := range mgr.ListKeys() {
			value, _ := mgr.GetValue(key)
			bold.Printf("%s: ", key)
			fmt.Println(value)
		}
		return nil
	},
}
```

## Acceptance Criteria

- [ ] `clerk config list` displays all configuration options with defaults
- [ ] `clerk config get region` returns the region value
- [ ] `clerk config set region us-west-2` updates the config file
- [ ] Config file is created at `$HOME/.clerk/config.json`
- [ ] Config file has proper permissions (0600)
- [ ] Invalid config keys return appropriate errors
- [ ] Duration parsing works for TTL values (e.g., "3h", "30m")
- [ ] Type validation works for `default_type`

## Notes

- Config file uses JSON format for simplicity and wide tooling support
- All config keys support both underscore and dot notation (e.g., `cache_ttl` or `cache.ttl`)
- Default values are applied when config file doesn't exist
- Config directory permissions are 0700, file permissions are 0600 for security
