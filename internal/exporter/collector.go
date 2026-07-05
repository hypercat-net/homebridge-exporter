package exporter

import (
	"context"
	"sync"
	"time"

	"github.com/hypercat-net/homebridge-exporter/internal/config"
	"github.com/hypercat-net/homebridge-exporter/internal/homebridge"
	"github.com/prometheus/client_golang/prometheus"
)

type configuredMetric struct {
	uniqueID       string
	label          string
	characteristic string
}

type cachedValue struct {
	value       float64
	serviceType string
	present     bool
}

// Collector polls Homebridge in the background and exposes cached Prometheus metrics.
type Collector struct {
	client       *homebridge.Client
	auth         *homebridge.AuthManager
	accessories  []config.AccessoryConfig
	pollInterval time.Duration

	mu sync.RWMutex

	metrics []configuredMetric
	labels  map[string]string
	cache   map[string]cachedValue
	accessoryFound map[string]bool

	up                float64
	lastScrape        time.Time
	scrapeDuration    float64
	scrapeErrors      float64
	lastPollSucceeded bool
	hasSuccessfulPoll bool
}

// NewCollector creates a collector and pre-registers configured metric series.
func NewCollector(client *homebridge.Client, auth *homebridge.AuthManager, accessories []config.AccessoryConfig, pollInterval time.Duration) *Collector {
	c := &Collector{
		client:       client,
		auth:         auth,
		accessories:  accessories,
		pollInterval: pollInterval,
		labels:       make(map[string]string),
		cache:        make(map[string]cachedValue),
		accessoryFound: make(map[string]bool),
	}

	for _, acc := range accessories {
		label := acc.Label
		if label == "" {
			label = acc.UniqueID
		}
		c.labels[acc.UniqueID] = label

		for _, ch := range acc.Characteristics {
			c.metrics = append(c.metrics, configuredMetric{
				uniqueID:       acc.UniqueID,
				label:          label,
				characteristic: ch,
			})
			key := cacheKey(acc.UniqueID, ch)
			c.cache[key] = cachedValue{}
		}
	}

	return c
}

// Start begins background polling until ctx is cancelled.
func (c *Collector) Start(ctx context.Context) {
	c.poll(ctx)

	ticker := time.NewTicker(c.pollInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			c.poll(ctx)
		}
	}
}

func (c *Collector) poll(ctx context.Context) {
	start := time.Now()

	err := c.scrapeOnce(ctx)
	duration := time.Since(start).Seconds()

	c.mu.Lock()
	defer c.mu.Unlock()

	c.lastScrape = start
	c.scrapeDuration = duration

	if err != nil {
		c.up = 0
		c.scrapeErrors++
		c.lastPollSucceeded = false
		return
	}

	c.up = 1
	c.lastPollSucceeded = true
	c.hasSuccessfulPoll = true
}

func (c *Collector) scrapeOnce(ctx context.Context) error {
	if err := c.auth.Refresh(ctx); err != nil {
		return err
	}

	all, err := c.client.ListAccessories(ctx)
	if err != nil {
		return err
	}

	uniqueIDs := make(map[string]struct{}, len(c.accessories))
	for _, acc := range c.accessories {
		uniqueIDs[acc.UniqueID] = struct{}{}
	}

	filtered := homebridge.FilterAccessories(all, uniqueIDs)
	byID := make(map[string]homebridge.Accessory, len(filtered))
	for _, acc := range filtered {
		byID[acc.UniqueID] = acc
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	accessoryFound := make(map[string]bool, len(byID))
	for id := range byID {
		accessoryFound[id] = true
	}

	for _, m := range c.metrics {
		key := cacheKey(m.uniqueID, m.characteristic)
		acc, found := byID[m.uniqueID]

		entry := c.cache[key]
		entry.present = false
		if found {
			entry.serviceType = acc.Type
			if v, ok := homebridge.NumericCharacteristicValue(acc.Values, m.characteristic); ok {
				entry.value = v
				entry.present = true
			}
		}

		c.cache[key] = entry
	}

	c.accessoryFound = accessoryFound

	for _, acc := range c.accessories {
		if apiAcc, ok := byID[acc.UniqueID]; ok && acc.Label == "" && apiAcc.ServiceName != "" {
			c.labels[acc.UniqueID] = apiAcc.ServiceName
		} else if acc.Label != "" {
			c.labels[acc.UniqueID] = acc.Label
		}
	}

	return nil
}

// PollOnce runs a single poll cycle synchronously.
func (c *Collector) PollOnce(ctx context.Context) {
	c.poll(ctx)
}

// Ready reports whether the exporter has polled successfully recently.
func (c *Collector) Ready() bool {
	c.mu.RLock()
	defer c.mu.RUnlock()

	if !c.hasSuccessfulPoll || !c.lastPollSucceeded {
		return false
	}

	maxAge := 2 * c.pollInterval
	return time.Since(c.lastScrape) <= maxAge
}

// Describe implements prometheus.Collector.
func (c *Collector) Describe(ch chan<- *prometheus.Desc) {
	ch <- characteristicValueDesc
	ch <- exporterUpDesc
	ch <- lastScrapeTimestampDesc
	ch <- scrapeDurationDesc
	ch <- scrapeErrorsDesc
	ch <- accessoryUpDesc
}

// Collect implements prometheus.Collector.
func (c *Collector) Collect(ch chan<- prometheus.Metric) {
	c.mu.RLock()
	defer c.mu.RUnlock()

	accessoryPresent := c.accessoryFound

	for _, m := range c.metrics {
		key := cacheKey(m.uniqueID, m.characteristic)
		entry := c.cache[key]
		label := c.labels[m.uniqueID]

		if entry.present {
			ch <- prometheus.MustNewConstMetric(
				characteristicValueDesc,
				prometheus.GaugeValue,
				entry.value,
				label,
				m.uniqueID,
				m.characteristic,
				entry.serviceType,
				characteristicUnit(m.characteristic),
			)
		}
	}

	seenAccessoryUp := make(map[string]struct{})
	for _, m := range c.metrics {
		label := c.labels[m.uniqueID]
		upKey := m.uniqueID + "\x00" + label
		if _, ok := seenAccessoryUp[upKey]; ok {
			continue
		}
		seenAccessoryUp[upKey] = struct{}{}

		up := 0.0
		if accessoryPresent[m.uniqueID] {
			up = 1
		}
		ch <- prometheus.MustNewConstMetric(
			accessoryUpDesc,
			prometheus.GaugeValue,
			up,
			m.uniqueID,
			label,
		)
	}

	ch <- prometheus.MustNewConstMetric(exporterUpDesc, prometheus.GaugeValue, c.up)

	if !c.lastScrape.IsZero() {
		ch <- prometheus.MustNewConstMetric(lastScrapeTimestampDesc, prometheus.GaugeValue, float64(c.lastScrape.Unix()))
	}
	ch <- prometheus.MustNewConstMetric(scrapeDurationDesc, prometheus.GaugeValue, c.scrapeDuration)
	ch <- prometheus.MustNewConstMetric(scrapeErrorsDesc, prometheus.CounterValue, c.scrapeErrors)
}

func cacheKey(uniqueID, characteristic string) string {
	return uniqueID + "\x00" + characteristic
}
