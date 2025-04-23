package main

import (
	"log/slog"

	"github.com/currantlabs/ble/linux"
	"gopkg.in/ini.v1"
)

// Config represents a configuration
type Config struct {
	Devices []Device
	Host    *linux.Device
}

// NewConfig returns a new Config
func NewConfig(file string) (*Config, error) {
	slog.Info("Loading configuration", "file", file)
	cfg, err := ini.Load(file)
	if err != nil {
		return &Config{}, err
	}

	sec, err := cfg.GetSection("Devices")
	if err != nil {
		return &Config{}, err
	}
	names := sec.KeyStrings()

	devices := []Device{}
	for i, name := range names {
		addr := sec.Key(name).String()
		slog.Info("Found device in config",
			"index", i,
			"device", name,
			"address", addr)
		devices = append(devices, Device{
			Name: name,
			Addr: addr,
		})
	}

	return &Config{
		Devices: devices,
	}, nil
}
