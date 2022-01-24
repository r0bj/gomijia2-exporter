package main

import (
	"log"

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
	log.Printf("[Config] Loading Configuration (%s)", file)
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
		log.Printf("[Config] Device %02d: %s (%s)", i, name, addr)
		devices = append(devices, Device{
			Name: name,
			Addr: addr,
		})
	}

	return &Config{
		Devices: devices,
	}, nil
}
