package main

import (
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/currantlabs/ble/linux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
)

const (
	ver string = "0.14"
	initialDelay int = 5
)

var (
	configFile          = flag.String("config_file", "config.ini", "Config file location")
	listenAddress       = flag.String("web.listen-address", ":9999", "Address to listen on for web interface and telemetry")
	measurementInterval = flag.Int("measurement-interval", 60, "Measurement interval in seconds")
	version             = flag.Bool("v", false, "Prints current version")
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

	if *version {
		fmt.Println(ver)
		os.Exit(0)
	}

	log.Printf("[main] Starting version %s", ver)
	// Sleep is a workaround for: can't init hci: no devices available: (hci0: can't up device: interrupted system call)
	time.Sleep(time.Duration(initialDelay) * time.Second)

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
		go RegisterHandler(device)
	}

	http.Handle("/metrics", promhttp.Handler())
	log.Fatal(http.ListenAndServe(*listenAddress, nil))
}
