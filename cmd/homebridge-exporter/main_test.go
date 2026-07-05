package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/hypercat-net/homebridge-exporter/internal/config"
	"github.com/hypercat-net/homebridge-exporter/internal/exporter"
	"github.com/hypercat-net/homebridge-exporter/internal/homebridge"
	"github.com/prometheus/client_golang/prometheus"
)

func TestValueKeys(t *testing.T) {
	keys := valueKeys(map[string]interface{}{
		"CurrentTemperature": 1.0,
		"StatusActive":       true,
	})
	if len(keys) != 2 || keys[0] != "CurrentTemperature" || keys[1] != "StatusActive" {
		t.Fatalf("valueKeys() = %v", keys)
	}
	if valueKeys(nil) != nil {
		t.Fatal("valueKeys(nil) should return nil")
	}
}

func TestHTTPHandlers(t *testing.T) {
	accessories := []config.AccessoryConfig{{
		UniqueID:        "abc",
		Characteristics: []string{"CurrentTemperature"},
	}}

	auth := homebridge.NewAuthManager("http://example", "a", "b", "", false, http.DefaultClient)
	client := homebridge.NewClient("http://example", auth, http.DefaultClient)
	collector := exporter.NewCollector(client, auth, accessories, 30*time.Second)
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	handler := newHTTPHandler(collector, registry)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	t.Run("health", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/health")
		if err != nil {
			t.Fatalf("GET /health error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /health status = %d, want 200", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "ok" {
			t.Fatalf("GET /health body = %q, want ok", body)
		}
	})

	t.Run("ready not ready", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/ready")
		if err != nil {
			t.Fatalf("GET /ready error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusServiceUnavailable {
			t.Fatalf("GET /ready status = %d, want 503", resp.StatusCode)
		}
	})

	t.Run("metrics", func(t *testing.T) {
		resp, err := http.Get(srv.URL + "/metrics")
		if err != nil {
			t.Fatalf("GET /metrics error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /metrics status = %d, want 200", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if len(body) == 0 {
			t.Fatal("GET /metrics returned empty body")
		}
		if !strings.Contains(string(body), "homebridge_exporter_up") {
			t.Fatalf("GET /metrics body missing homebridge_exporter_up")
		}
	})

	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/noauth":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "token"})
		case "/api/accessories":
			json.NewEncoder(w).Encode([]homebridge.Accessory{{
				UniqueID:    "abc",
				ServiceName: "Test",
				Type:        "Sensor",
				Values:      map[string]interface{}{"CurrentTemperature": 1.0},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	readyAuth := homebridge.NewAuthManager(mock.URL, "", "", "", true, mock.Client())
	readyClient := homebridge.NewClient(mock.URL, readyAuth, mock.Client())
	readyCollector := exporter.NewCollector(readyClient, readyAuth, accessories, 30*time.Second)
	readyCollector.PollOnce(context.Background())

	readyRegistry := prometheus.NewRegistry()
	readyRegistry.MustRegister(readyCollector)
	readySrv := httptest.NewServer(newHTTPHandler(readyCollector, readyRegistry))
	defer readySrv.Close()

	t.Run("ready", func(t *testing.T) {
		resp, err := http.Get(readySrv.URL + "/ready")
		if err != nil {
			t.Fatalf("GET /ready error = %v", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			t.Fatalf("GET /ready status = %d, want 200", resp.StatusCode)
		}
		body, err := io.ReadAll(resp.Body)
		if err != nil {
			t.Fatal(err)
		}
		if string(body) != "ready" {
			t.Fatalf("GET /ready body = %q, want ready", body)
		}
	})
}

func TestHTTPReadyStale(t *testing.T) {
	mock := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/noauth":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "token"})
		case "/api/accessories":
			json.NewEncoder(w).Encode([]homebridge.Accessory{{
				UniqueID: "abc",
				Values:   map[string]interface{}{"CurrentTemperature": 1.0},
			}})
		default:
			http.NotFound(w, r)
		}
	}))
	defer mock.Close()

	accessories := []config.AccessoryConfig{{
		UniqueID:        "abc",
		Characteristics: []string{"CurrentTemperature"},
	}}
	auth := homebridge.NewAuthManager(mock.URL, "", "", "", true, mock.Client())
	client := homebridge.NewClient(mock.URL, auth, mock.Client())
	collector := exporter.NewCollector(client, auth, accessories, 10*time.Millisecond)
	collector.PollOnce(context.Background())
	time.Sleep(25 * time.Millisecond)

	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)
	srv := httptest.NewServer(newHTTPHandler(collector, registry))
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/ready")
	if err != nil {
		t.Fatalf("GET /ready error = %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Fatalf("GET /ready status = %d, want 503 for stale poll", resp.StatusCode)
	}
}
