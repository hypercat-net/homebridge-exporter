package config

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestLoadAccessoriesFromFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accessories.yaml")
	content := `accessories:
  - unique_id: fridge-id
    label: fridge
    characteristics:
      - CurrentTemperature
  - unique_id: freezer-id
    characteristics:
      - CurrentTemperature
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	file, err := LoadAccessoriesFile(path)
	if err != nil {
		t.Fatalf("LoadAccessoriesFile: %v", err)
	}
	if len(file.Accessories) != 2 {
		t.Fatalf("expected 2 accessories, got %d", len(file.Accessories))
	}
	if file.Accessories[0].UniqueID != "fridge-id" || file.Accessories[0].Label != "fridge" {
		t.Fatalf("unexpected first accessory: %+v", file.Accessories[0])
	}
	if file.Accessories[1].Label != "" {
		t.Fatalf("expected empty label for second accessory, got %q", file.Accessories[1].Label)
	}
}

func TestLoadFromEnv(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accessories.yaml")
	content := `accessories:
  - unique_id: test-id
    label: test
    characteristics:
      - CurrentTemperature
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOMEBRIDGE_URL", "http://127.0.0.1:8581")
	t.Setenv("HOMEBRIDGE_NOAUTH", "true")
	t.Setenv("EXPORTER_CONFIG_PATH", path)
	t.Setenv("EXPORTER_LISTEN_ADDR", ":9191")
	t.Setenv("EXPORTER_POLL_INTERVAL", "15s")
	t.Setenv("EXPORTER_REQUEST_TIMEOUT", "5s")

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load: %v", err)
	}

	if cfg.HomebridgeURL != "http://127.0.0.1:8581" {
		t.Fatalf("unexpected url: %s", cfg.HomebridgeURL)
	}
	if !cfg.HomebridgeNoAuth {
		t.Fatal("expected noauth=true")
	}
	if cfg.ListenAddr != ":9191" {
		t.Fatalf("unexpected listen addr: %s", cfg.ListenAddr)
	}
	if cfg.PollInterval != 15*time.Second {
		t.Fatalf("unexpected poll interval: %s", cfg.PollInterval)
	}
	if cfg.RequestTimeout != 5*time.Second {
		t.Fatalf("unexpected request timeout: %s", cfg.RequestTimeout)
	}
	if len(cfg.Accessories.Accessories) != 1 || cfg.Accessories.Accessories[0].UniqueID != "test-id" {
		t.Fatalf("unexpected accessories: %+v", cfg.Accessories.Accessories)
	}
}

func TestValidateRequiresCredentials(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accessories.yaml")
	content := `accessories:
  - unique_id: test-id
    characteristics:
      - CurrentTemperature
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOMEBRIDGE_NOAUTH", "false")
	t.Setenv("HOMEBRIDGE_USERNAME", "")
	t.Setenv("HOMEBRIDGE_PASSWORD", "")
	t.Setenv("EXPORTER_CONFIG_PATH", path)

	if _, err := Load(); err == nil {
		t.Fatal("expected error when credentials missing")
	}
}

func TestValidateRequiresUniqueID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accessories.yaml")
	content := `accessories:
  - label: bad
    characteristics:
      - CurrentTemperature
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOMEBRIDGE_NOAUTH", "true")
	t.Setenv("EXPORTER_CONFIG_PATH", path)

	if _, err := Load(); err == nil {
		t.Fatal("expected error when unique_id missing")
	}
}

func TestLoadConnection(t *testing.T) {
	t.Setenv("HOMEBRIDGE_URL", "http://example:8581")
	t.Setenv("HOMEBRIDGE_NOAUTH", "true")
	t.Setenv("EXPORTER_POLL_INTERVAL", "45s")

	conn, err := LoadConnection()
	if err != nil {
		t.Fatalf("LoadConnection: %v", err)
	}
	if conn.HomebridgeURL != "http://example:8581" {
		t.Fatalf("unexpected url: %s", conn.HomebridgeURL)
	}
	if conn.PollInterval != 45*time.Second {
		t.Fatalf("unexpected poll interval: %s", conn.PollInterval)
	}
}

func TestLoadConnectionInvalidPollInterval(t *testing.T) {
	t.Setenv("HOMEBRIDGE_NOAUTH", "true")
	t.Setenv("EXPORTER_POLL_INTERVAL", "not-a-duration")

	_, err := LoadConnection()
	if err == nil {
		t.Fatal("expected error for invalid EXPORTER_POLL_INTERVAL")
	}
}

func TestValidateDuplicateUniqueID(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accessories.yaml")
	content := `accessories:
  - unique_id: dup-id
    characteristics:
      - CurrentTemperature
  - unique_id: dup-id
    characteristics:
      - CurrentTemperature
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOMEBRIDGE_NOAUTH", "true")
	t.Setenv("EXPORTER_CONFIG_PATH", path)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for duplicate unique_id")
	}
}

func TestValidateRequiresCharacteristics(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "accessories.yaml")
	content := `accessories:
  - unique_id: test-id
    label: no-chars
`
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	t.Setenv("HOMEBRIDGE_NOAUTH", "true")
	t.Setenv("EXPORTER_CONFIG_PATH", path)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error when characteristics missing")
	}
}

func TestLoadUnreadableConfigFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "missing.yaml")

	t.Setenv("HOMEBRIDGE_NOAUTH", "true")
	t.Setenv("EXPORTER_CONFIG_PATH", path)

	_, err := Load()
	if err == nil {
		t.Fatal("expected error for unreadable config file")
	}
}
