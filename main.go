package main

import (
	"log/slog"
	"net/http"
	"os"
	"sync"
	"time"

	"github.com/currantlabs/ble/linux"
	"github.com/prometheus/client_golang/prometheus"
	"github.com/prometheus/client_golang/prometheus/promauto"
	"github.com/prometheus/client_golang/prometheus/promhttp"
	flag "github.com/spf13/pflag"
)

const (
	ver string = "0.36"
)

var (
	configFile          = flag.String("config-file", "config.ini", "Config file location")
	listenAddress       = flag.String("web.listen-address", ":8080", "Address to listen on for web interface and telemetry")
	measurementInterval = flag.Int("measurement-interval", 60, "Measurement interval in seconds")
	verbose             = flag.Bool("verbose", false, "Enable verbose output")
)

var (
	deviceErrorsCounter = promauto.NewCounterVec(prometheus.CounterOpts{
		Name: "mi_device_errors_total",
		Help: "MI device errors",
	},
		[]string{"location"})
)

// Global BLE device and mutex for synchronization
var (
	bleMutex            sync.Mutex
	bleDevice           *linux.Device
	resetBLEDeviceMutex sync.Mutex
	globalConfig        *Config // Store config globally for device reset
)

// resetBLEDevice recreates the BLE device to recover from persistent errors
func resetBLEDevice() error {
	resetBLEDeviceMutex.Lock()
	defer resetBLEDeviceMutex.Unlock()

	// Acquire the BLE device mutex to ensure no one is using it
	slog.Warn("Starting BLE device reset process")

	// Use a timeout when acquiring the mutex to prevent deadlocks
	timeout := time.After(10 * time.Second)
	done := make(chan bool, 1)

	// Try to acquire the mutex in a separate goroutine
	go func() {
		bleMutex.Lock()
		done <- true
	}()

	// Wait for either mutex acquisition or timeout
	select {
	case <-done:
		// Mutex was acquired successfully
		slog.Info("Acquired BLE mutex for device reset")
		defer bleMutex.Unlock()
	case <-timeout:
		slog.Error("Timeout waiting for BLE mutex during device reset, forcing reset")
		// If we can't acquire the mutex, the system might be in a deadlock
		if bleDevice != nil {
			slog.Info("Force stopping existing BLE device")
			bleDevice.Stop()
			bleDevice = nil
		}

		slog.Info("Creating new BLE device after timeout")
		var err error
		bleDevice, err = linux.NewDevice()
		if err != nil {
			slog.Error("Failed to create new BLE device after timeout", "error", err)
			return err
		}

		slog.Info("BLE device reset completed after timeout")
		ClearBLEDeviceResetRequest()
		return nil
	}

	// Reset all device error counters
	if globalConfig != nil {
		for _, device := range globalConfig.Devices {
			ResetErrors(device.Name)
			slog.Info("Reset error counter during device reset", "device", device.Name)
		}
	} else {
		slog.Warn("No global config available, skipping device error counter reset")
	}

	// Clean up existing device if it exists
	if bleDevice != nil {
		slog.Info("Stopping existing BLE device")
		bleDevice.Stop()
		bleDevice = nil
	}

	// Create new device
	slog.Info("Creating new BLE device")
	var err error
	bleDevice, err = linux.NewDevice()
	if err != nil {
		slog.Error("Failed to create new BLE device", "error", err)
		return err
	}

	slog.Info("BLE device reset completed successfully")
	ClearBLEDeviceResetRequest()
	return nil
}

func main() {
	var loggingLevel = new(slog.LevelVar)
	logger := slog.New(slog.NewJSONHandler(os.Stdout, &slog.HandlerOptions{Level: loggingLevel}))
	slog.SetDefault(logger)

	flag.Parse()

	if *verbose {
		loggingLevel.Set(slog.LevelDebug)
		slog.Debug("Debug logging enabled")
	}

	slog.Info("Starting", "version", ver)

	slog.Info("Reading configuration")
	config, err := NewConfig(*configFile)
	if err != nil {
		slog.Error("Unable to parse configuration", "error", err)
		os.Exit(1)
	}

	// Store config globally for device reset
	globalConfig = config

	// Create the BLE device once for all handlers to share
	slog.Info("Starting Linux Device")
	bleDevice, err = linux.NewDevice()
	if err != nil {
		slog.Error("Failed to initialize BLE device", "error", err)
		os.Exit(1)
	}

	// Start a goroutine to monitor and reset BLE device if needed
	go func() {
		checkInterval := 5 * time.Second // Reduced from 15 seconds

		for {
			if IsBLEDeviceResetRequested() {
				slog.Info("BLE device reset requested, attempting reset")
				if err := resetBLEDevice(); err != nil {
					slog.Error("BLE device reset failed", "error", err)
					// If reset fails, wait a bit longer before trying again
					time.Sleep(10 * time.Second) // Reduced from 30 seconds
				} else {
					slog.Info("BLE device reset successful")
				}
			}

			time.Sleep(checkInterval)
		}
	}()

	// Start handlers for each device with staggered timing
	for i, device := range config.Devices {
		slog.Info("Starting handler for device",
			"device", device.Name,
			"address", device.Addr)
		// Stagger the start times to avoid collisions
		startDelay := i * 10
		go func(d Device, delay int) {
			// Initial delay to stagger device polling
			time.Sleep(time.Duration(delay) * time.Second)
			RegisterHandler(d)
		}(device, startDelay)
	}

	slog.Info("Starting HTTP server", "address", *listenAddress)
	http.Handle("/metrics", promhttp.Handler())
	err = http.ListenAndServe(*listenAddress, nil)
	if err != nil {
		slog.Error("HTTP server error", "error", err)
		os.Exit(1)
	}
}
