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
	case "search_slash_prefix", "search.slash.prefix":
		return strconv.FormatBool(m.config.SearchSlashPrefix), nil
	case "describe_page_size", "describe.page.size":
		return strconv.Itoa(int(m.config.DescribePageSize)), nil
	case "describe_max_items", "describe.max.items":
		return strconv.Itoa(int(m.config.DescribeMaxItems)), nil
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
	case "search_slash_prefix", "search.slash.prefix":
		b, err := strconv.ParseBool(value)
		if err != nil {
			return fmt.Errorf("invalid boolean value for search_slash_prefix: %w", err)
		}
		m.config.SearchSlashPrefix = b
	case "describe_page_size", "describe.page.size":
		n, err := strconv.Atoi(value)
		if err != nil || n < 1 || n > 50 {
			return fmt.Errorf("describe_page_size must be between 1 and 50")
		}
		m.config.DescribePageSize = int32(n)
	case "describe_max_items", "describe.max.items":
		n, err := strconv.Atoi(value)
		if err != nil || n < 0 {
			return fmt.Errorf("describe_max_items must be >= 0 (0 = unlimited)")
		}
		m.config.DescribeMaxItems = int32(n)
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
		"search_slash_prefix",
		"describe_page_size",
		"describe_max_items",
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
