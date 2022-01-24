package main

import (
	"flag"
	"log"
	"net/http"

	"github.com/currantlabs/ble/linux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

var (
	configFile = flag.String("config_file", "config.ini", "Config file location")
)

var (
	deviceConnectionFailed = promauto.NewGaugeVec(prometheus.GaugeOpts{
		Name: "mi_device_connection_failed",
		Help: "MI device connection failed",
	},
	[]string{"location"})
)

func main() {
	flag.Parse()

	log.Print("[main] Reading configuration")
	config, err := NewConfig(*configFile)
	if err != nil {
		log.Fatal("Unable to parse configuration")
	}

	log.Print("[main] Starting Linux Device")
	config.Host, err = linux.NewDevice()
	if err != nil {
		log.Fatal(err)
	}

	for _, device := range config.Devices {
		log.Printf("[main:%s] Dialing (%s)", device.Name, device.Addr)
		if err := device.Connect(config.Host); err != nil {
			log.Printf("[main:%s] Failed to connect to device", device.Name)
			deviceConnectionFailed.WithLabelValues(device.Name).Set(1)
			continue
		} else {
			deviceConnectionFailed.WithLabelValues(device.Name).Set(0)
		}

		log.Printf("[main:%s] Registering handler", device.Name)
		device.RegisterHandler()
	}

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(":9999", nil))
}
