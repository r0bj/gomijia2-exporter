package main

import (
	"encoding/hex"
	"log"

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
	}
}
