package main

import (
	"context"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"sort"
	"strings"
	"syscall"
	"time"

	"github.com/hypercat-net/homebridge-exporter/internal/config"
	"github.com/hypercat-net/homebridge-exporter/internal/exporter"
	"github.com/hypercat-net/homebridge-exporter/internal/homebridge"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

func main() {
	listAccessories := flag.Bool("list-accessories", false, "List Homebridge accessories and exit")
	flag.Parse()

	conn, err := config.LoadConnection()
	if err != nil {
		log.Fatalf("load connection config: %v", err)
	}

	httpClient := &http.Client{Timeout: conn.RequestTimeout}
	auth := homebridge.NewAuthManager(
		conn.HomebridgeURL,
		conn.HomebridgeUser,
		conn.HomebridgePass,
		conn.HomebridgeOTP,
		conn.HomebridgeNoAuth,
		httpClient,
	)
	client := homebridge.NewClient(conn.HomebridgeURL, auth, httpClient)

	if *listAccessories {
		if err := runListAccessories(client); err != nil {
			log.Fatalf("list accessories: %v", err)
		}
		return
	}

	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("load accessories config: %v", err)
	}

	collector := exporter.NewCollector(client, auth, cfg.Accessories.Accessories, cfg.PollInterval)
	registry := prometheus.NewRegistry()
	registry.MustRegister(collector)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go collector.Start(ctx)

	server := &http.Server{
		Addr:              cfg.ListenAddr,
		Handler:           newHTTPHandler(collector, registry),
		ReadHeaderTimeout: 5 * time.Second,
	}

	go func() {
		log.Printf("listening on %s", cfg.ListenAddr)
		if err := server.ListenAndServe(); err != nil && err != http.ErrServerClosed {
			log.Fatalf("server error: %v", err)
		}
	}()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, syscall.SIGINT, syscall.SIGTERM)
	<-sigCh

	cancel()
	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer shutdownCancel()
	_ = server.Shutdown(shutdownCtx)
}

func newHTTPHandler(collector *exporter.Collector, registry *prometheus.Registry) http.Handler {
	mux := http.NewServeMux()
	mux.Handle("/metrics", promhttp.HandlerFor(registry, promhttp.HandlerOpts{}))
	mux.HandleFunc("/health", func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	mux.HandleFunc("/ready", func(w http.ResponseWriter, _ *http.Request) {
		if collector.Ready() {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("ready"))
			return
		}
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte("not ready"))
	})
	return mux
}

func runListAccessories(client *homebridge.Client) error {
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	accessories, err := client.ListAccessories(ctx)
	if err != nil {
		return err
	}

	sort.Slice(accessories, func(i, j int) bool {
		if accessories[i].ServiceName == accessories[j].ServiceName {
			return accessories[i].UniqueID < accessories[j].UniqueID
		}
		return accessories[i].ServiceName < accessories[j].ServiceName
	})

	fmt.Printf("%-36s  %-24s  %-22s  %s\n", "uniqueId", "serviceName", "type", "values keys")
	fmt.Printf("%-36s  %-24s  %-22s  %s\n", strings.Repeat("-", 36), strings.Repeat("-", 24), strings.Repeat("-", 22), strings.Repeat("-", 20))

	for _, acc := range accessories {
		keys := valueKeys(acc.Values)
		fmt.Printf("%-36s  %-24s  %-22s  %s\n", acc.UniqueID, acc.ServiceName, acc.Type, strings.Join(keys, ", "))
	}
	return nil
}

func valueKeys(values map[string]interface{}) []string {
	if len(values) == 0 {
		return nil
	}
	keys := make([]string, 0, len(values))
	for k := range values {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
