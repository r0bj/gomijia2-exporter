package main

import (
	"encoding/hex"
	"log"
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
			log.Printf("[handler:%s] Unable to unmarshal data (%s)", name, s)
		}
		log.Printf("[handler:%s] %s (%s)", name, r.String(), s)
		temperature.WithLabelValues(name).Set(r.Temperature)
		humidity.WithLabelValues(name).Set(r.Humidity)
		voltage.WithLabelValues(name).Set(r.Voltage)
		// 3.1V or above --> 100% 2.1V --> 0 %
		battery.WithLabelValues(name).Set(math.Round(math.Min((r.Voltage-2.1) * 100, 100) * 100) / 100)
	}
}
