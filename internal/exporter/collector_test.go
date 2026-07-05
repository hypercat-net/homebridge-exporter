package exporter

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/hypercat-net/homebridge-exporter/internal/config"
	"github.com/hypercat-net/homebridge-exporter/internal/homebridge"
	"github.com/prometheus/client_golang/prometheus"
	dto "github.com/prometheus/client_model/go"
)

func TestCollectorScrapeAndMetrics(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login", "/api/auth/refresh":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "token"})
		case "/api/accessories":
			json.NewEncoder(w).Encode([]homebridge.Accessory{
				{
					UniqueID:    "fridge-id",
					ServiceName: "Fridge",
					Type:        "TemperatureSensor",
					Values:      map[string]interface{}{"CurrentTemperature": 4.2},
				},
				{
					UniqueID: "other-id",
					Values:   map[string]interface{}{"CurrentTemperature": 99.0},
				},
			})
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	accessories := []config.AccessoryConfig{{
		UniqueID:        "fridge-id",
		Label:           "fridge",
		Characteristics: []string{"CurrentTemperature"},
	}}

	auth := homebridge.NewAuthManager(srv.URL, "admin", "secret", "", false, srv.Client())
	client := homebridge.NewClient(srv.URL, auth, srv.Client())
	collector := NewCollector(client, auth, accessories, 30*time.Second)

	if err := collector.scrapeOnce(context.Background()); err != nil {
		t.Fatalf("scrapeOnce() error = %v", err)
	}

	collector.mu.Lock()
	collector.up = 1
	collector.lastPollSucceeded = true
	collector.hasSuccessfulPoll = true
	collector.lastScrape = time.Now()
	collector.mu.Unlock()

	if !collector.Ready() {
		t.Fatal("Ready() = false, want true after successful scrape")
	}

	metrics := gatherCollectorMetrics(t, collector)

	value := findMetricValue(t, metrics, "homebridge_characteristic_value", map[string]string{
		"accessory":      "fridge",
		"unique_id":      "fridge-id",
		"characteristic": "CurrentTemperature",
		"service_type":   "TemperatureSensor",
		"unit":           "celsius",
	})
	if value != 4.2 {
		t.Fatalf("characteristic value = %v, want 4.2", value)
	}

	accessoryUp := findMetricValue(t, metrics, "homebridge_exporter_accessory_up", map[string]string{
		"unique_id": "fridge-id",
		"accessory": "fridge",
	})
	if accessoryUp != 1 {
		t.Fatalf("accessory_up = %v, want 1", accessoryUp)
	}

	exporterUp := findMetricValue(t, metrics, "homebridge_exporter_up", nil)
	if exporterUp != 1 {
		t.Fatalf("exporter_up = %v, want 1", exporterUp)
	}
}

func TestCollectorReadyAfterStalePoll(t *testing.T) {
	auth := homebridge.NewAuthManager("http://example", "a", "b", "", false, http.DefaultClient)
	client := homebridge.NewClient("http://example", auth, http.DefaultClient)
	collector := NewCollector(client, auth, []config.AccessoryConfig{{
		UniqueID:        "abc",
		Characteristics: []string{"CurrentTemperature"},
	}}, 30*time.Second)

	collector.mu.Lock()
	collector.hasSuccessfulPoll = true
	collector.lastPollSucceeded = true
	collector.lastScrape = time.Now().Add(-61 * time.Second)
	collector.mu.Unlock()

	if collector.Ready() {
		t.Fatal("Ready() = true, want false when last scrape is stale")
	}
}

func TestCharacteristicUnit(t *testing.T) {
	tests := []struct {
		name           string
		characteristic string
		want           string
	}{
		{name: "celsius", characteristic: "CurrentTemperature", want: "celsius"},
		{name: "humidity", characteristic: "RelativeHumidity", want: "percent"},
		{name: "battery", characteristic: "BatteryLevel", want: "percent"},
		{name: "lux", characteristic: "CurrentAmbientLightLevel", want: "lux"},
		{name: "ppm", characteristic: "CarbonDioxideLevel", want: "ppm"},
		{name: "degrees", characteristic: "CurrentHorizontalTiltAngle", want: "degrees"},
		{name: "unknown", characteristic: "UnknownCharacteristic", want: ""},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := characteristicUnit(tt.characteristic); got != tt.want {
				t.Fatalf("characteristicUnit(%q) = %q, want %q", tt.characteristic, got, tt.want)
			}
		})
	}
}

func TestCollectorPollFailure(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/auth/login", "/api/auth/refresh":
			json.NewEncoder(w).Encode(map[string]string{"access_token": "token"})
		case "/api/accessories":
			http.Error(w, "internal error", http.StatusInternalServerError)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	accessories := []config.AccessoryConfig{{
		UniqueID:        "fridge-id",
		Characteristics: []string{"CurrentTemperature"},
	}}

	auth := homebridge.NewAuthManager(srv.URL, "admin", "secret", "", false, srv.Client())
	client := homebridge.NewClient(srv.URL, auth, srv.Client())
	collector := NewCollector(client, auth, accessories, 30*time.Second)

	collector.PollOnce(context.Background())

	collector.mu.RLock()
	up := collector.up
	scrapeErrors := collector.scrapeErrors
	collector.mu.RUnlock()

	if up != 0 {
		t.Fatalf("up = %v, want 0", up)
	}
	if scrapeErrors != 1 {
		t.Fatalf("scrapeErrors = %v, want 1", scrapeErrors)
	}
	if collector.Ready() {
		t.Fatal("Ready() = true, want false after failed poll")
	}
}

func gatherCollectorMetrics(t *testing.T, collector *Collector) []*dto.MetricFamily {
	t.Helper()

	reg := prometheus.NewRegistry()
	reg.MustRegister(collector)

	out, err := reg.Gather()
	if err != nil {
		t.Fatalf("Gather() error = %v", err)
	}
	return out
}

func findMetricValue(t *testing.T, families []*dto.MetricFamily, name string, labels map[string]string) float64 {
	t.Helper()

	for _, family := range families {
		if family.GetName() != name {
			continue
		}
		for _, metric := range family.GetMetric() {
			if labelsMatch(metric, labels) {
				if metric.Gauge != nil {
					return metric.GetGauge().GetValue()
				}
			}
		}
	}

	t.Fatalf("metric %q with labels %v not found", name, labels)
	return 0
}

func labelsMatch(metric *dto.Metric, expected map[string]string) bool {
	if len(expected) == 0 {
		return len(metric.GetLabel()) == 0
	}

	found := make(map[string]string, len(metric.GetLabel()))
	for _, lp := range metric.GetLabel() {
		found[lp.GetName()] = lp.GetValue()
	}

	for k, v := range expected {
		if found[k] != v {
			return false
		}
	}
	return true
}

