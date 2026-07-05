package exporter

import (
	"github.com/prometheus/client_golang/prometheus"
)

const (
	namespace = "homebridge"
)

var (
	characteristicValueDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "", "characteristic_value"),
		"Current numeric value of a Homebridge accessory characteristic.",
		[]string{"accessory", "unique_id", "characteristic", "service_type", "unit"},
		nil,
	)

	exporterUpDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "exporter", "up"),
		"1 if the last poll cycle succeeded, 0 otherwise.",
		nil,
		nil,
	)

	lastScrapeTimestampDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "exporter", "last_scrape_timestamp_seconds"),
		"Unix timestamp of the last completed poll cycle.",
		nil,
		nil,
	)

	scrapeDurationDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "exporter", "scrape_duration_seconds"),
		"Duration of the last poll cycle in seconds.",
		nil,
		nil,
	)

	scrapeErrorsDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "exporter", "scrape_errors_total"),
		"Total number of failed poll cycles.",
		nil,
		nil,
	)

	accessoryUpDesc = prometheus.NewDesc(
		prometheus.BuildFQName(namespace, "exporter", "accessory_up"),
		"1 if the accessory was present in the last successful poll, 0 otherwise.",
		[]string{"unique_id", "accessory"},
		nil,
	)
)

// characteristicUnit returns a Prometheus unit label for known characteristics.
func characteristicUnit(name string) string {
	switch name {
	case "CurrentTemperature", "TargetTemperature", "CoolingThresholdTemperature", "HeatingThresholdTemperature":
		return "celsius"
	case "RelativeHumidity", "CurrentRelativeHumidity":
		return "percent"
	case "CurrentAmbientLightLevel":
		return "lux"
	case "CurrentHorizontalTiltAngle", "CurrentVerticalTiltAngle", "TargetHorizontalTiltAngle", "TargetVerticalTiltAngle":
		return "degrees"
	case "BatteryLevel":
		return "percent"
	case "CarbonDioxideLevel", "CarbonMonoxideLevel", "NitrogenDioxideDensity", "OzoneDensity", "PM2_5Density", "PM10Density", "SulphurDioxideDensity", "VOCDensity":
		return "ppm"
	case "CurrentAirPressure":
		return "hpa"
	default:
		return ""
	}
}
