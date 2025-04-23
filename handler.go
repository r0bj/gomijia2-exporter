package main

import (
	"encoding/hex"
	"log/slog"
	"math"

	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
)

var (
	temperature = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mi_temperature",
		Help: "MI sensor temperature",
	},
		[]string{"location"})
	humidity = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mi_humidity",
		Help: "MI sensor humidity",
	},
		[]string{"location"})
	voltage = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mi_voltage",
		Help: "MI sensor battery voltage",
	},
		[]string{"location"})
	battery = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mi_battery",
		Help: "MI sensor battery level",
	},
		[]string{"location"})
)

func handlerPublisher(name string) func(req []byte) {
	return func(req []byte) {
		s := hex.EncodeToString(req)
		r, err := Unmarshall(req)
		if err != nil {
			slog.Error("Unable to unmarshal data",
				"device", name,
				"data", s,
				"error", err)
			return
		}

		slog.Info("Received sensor data",
			"device", name,
			"temperature", r.Temperature,
			"humidity", r.Humidity,
			"voltage", r.Voltage,
			"rawData", s)

		temperature.WithLabelValues(name).Set(r.Temperature)
		humidity.WithLabelValues(name).Set(r.Humidity)
		voltage.WithLabelValues(name).Set(r.Voltage)
		// 3.1V or above --> 100% 2.1V --> 0 %
		batteryPercent := math.Round(math.Min((r.Voltage-2.1)*100, 100)*100) / 100
		battery.WithLabelValues(name).Set(batteryPercent)

		slog.Info("Updated metrics",
			"device", name,
			"batteryPercent", batteryPercent)
	}
}
