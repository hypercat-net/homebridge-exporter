package config

import (
	"fmt"
	"os"
	"strconv"
	"time"

	"gopkg.in/yaml.v3"
)

// Connection holds Homebridge and exporter connection settings from environment.
type Connection struct {
	HomebridgeURL     string
	HomebridgeUser    string
	HomebridgePass    string
	HomebridgeOTP     string
	HomebridgeNoAuth  bool
	ListenAddr        string
	ConfigPath        string
	PollInterval      time.Duration
	RequestTimeout    time.Duration
}

// AccessoryConfig describes one accessory to export metrics for.
type AccessoryConfig struct {
	UniqueID         string   `yaml:"unique_id"`
	Label            string   `yaml:"label"`
	Characteristics  []string `yaml:"characteristics"`
}

// AccessoriesFile is the YAML accessories configuration.
type AccessoriesFile struct {
	Accessories []AccessoryConfig `yaml:"accessories"`
}

// Config combines connection settings and accessory selection.
type Config struct {
	Connection
	Accessories AccessoriesFile
}

// LoadConnection reads connection settings from environment variables.
func LoadConnection() (Connection, error) {
	conn, err := loadConnectionFromEnv()
	if err != nil {
		return Connection{}, err
	}
	if err := validateConnection(conn); err != nil {
		return Connection{}, err
	}
	return conn, nil
}

// Load reads environment variables and the accessories YAML file.
func Load() (*Config, error) {
	conn, err := loadConnectionFromEnv()
	if err != nil {
		return nil, err
	}
	if err := validateConnection(conn); err != nil {
		return nil, err
	}

	accessories, err := LoadAccessoriesFile(conn.ConfigPath)
	if err != nil {
		return nil, err
	}

	if err := validateAccessories(accessories); err != nil {
		return nil, err
	}

	return &Config{
		Connection:  conn,
		Accessories: accessories,
	}, nil
}

func validateConnection(conn Connection) error {
	if conn.HomebridgeNoAuth {
		return nil
	}
	if conn.HomebridgeUser == "" || conn.HomebridgePass == "" {
		return fmt.Errorf("HOMEBRIDGE_USERNAME and HOMEBRIDGE_PASSWORD are required when HOMEBRIDGE_NOAUTH is false")
	}
	return nil
}

// LoadAccessoriesFile parses accessories YAML from path.
func LoadAccessoriesFile(path string) (AccessoriesFile, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return AccessoriesFile{}, fmt.Errorf("read accessories config %q: %w", path, err)
	}

	var file AccessoriesFile
	if err := yaml.Unmarshal(data, &file); err != nil {
		return AccessoriesFile{}, fmt.Errorf("parse accessories config %q: %w", path, err)
	}

	return file, nil
}

func loadConnectionFromEnv() (Connection, error) {
	pollInterval, err := parseDurationEnv("EXPORTER_POLL_INTERVAL", 30*time.Second)
	if err != nil {
		return Connection{}, err
	}

	requestTimeout, err := parseDurationEnv("EXPORTER_REQUEST_TIMEOUT", 10*time.Second)
	if err != nil {
		return Connection{}, err
	}

	noAuth, err := parseBoolEnv("HOMEBRIDGE_NOAUTH", false)
	if err != nil {
		return Connection{}, err
	}

	conn := Connection{
		HomebridgeURL:    envOrDefault("HOMEBRIDGE_URL", "http://homebridge:8581"),
		HomebridgeUser:   os.Getenv("HOMEBRIDGE_USERNAME"),
		HomebridgePass:   os.Getenv("HOMEBRIDGE_PASSWORD"),
		HomebridgeOTP:    os.Getenv("HOMEBRIDGE_OTP"),
		HomebridgeNoAuth: noAuth,
		ListenAddr:       envOrDefault("EXPORTER_LISTEN_ADDR", ":9090"),
		ConfigPath:       envOrDefault("EXPORTER_CONFIG_PATH", "/config/accessories.yaml"),
		PollInterval:     pollInterval,
		RequestTimeout:   requestTimeout,
	}

	if conn.HomebridgeURL == "" {
		return Connection{}, fmt.Errorf("HOMEBRIDGE_URL must not be empty")
	}
	if !conn.HomebridgeNoAuth && (conn.HomebridgeUser == "" || conn.HomebridgePass == "") {
		return Connection{}, fmt.Errorf("HOMEBRIDGE_USERNAME and HOMEBRIDGE_PASSWORD are required unless HOMEBRIDGE_NOAUTH=true")
	}

	return conn, nil
}

func validateAccessories(file AccessoriesFile) error {
	if len(file.Accessories) == 0 {
		return fmt.Errorf("accessories config must contain at least one accessory")
	}

	seen := make(map[string]struct{}, len(file.Accessories))
	for i, acc := range file.Accessories {
		if acc.UniqueID == "" {
			return fmt.Errorf("accessories[%d]: unique_id is required", i)
		}
		if _, ok := seen[acc.UniqueID]; ok {
			return fmt.Errorf("accessories[%d]: duplicate unique_id %q", i, acc.UniqueID)
		}
		seen[acc.UniqueID] = struct{}{}

		if len(acc.Characteristics) == 0 {
			return fmt.Errorf("accessories[%d]: at least one characteristic is required", i)
		}
	}

	return nil
}

func envOrDefault(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func parseDurationEnv(key string, fallback time.Duration) (time.Duration, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return 0, fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	return d, nil
}

func parseBoolEnv(key string, fallback bool) (bool, error) {
	v := os.Getenv(key)
	if v == "" {
		return fallback, nil
	}
	b, err := strconv.ParseBool(v)
	if err != nil {
		return false, fmt.Errorf("invalid %s %q: %w", key, v, err)
	}
	return b, nil
}
