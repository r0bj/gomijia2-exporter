package main

import (
	"encoding/binary"
	"fmt"
	"log"
)

// Reading represents a Temperature|Humidity readings
type Reading struct {
	Temperature float64
	Humidity    float64
	Voltage     float64
}

// ToString converts a Reading to a string
func (r *Reading) String() string {
	return fmt.Sprintf("Temperature: %.04f; Humidity: %.04f; Voltge: %.04f", r.Temperature, r.Humidity, r.Voltage)
}

// Unmarshall converts an encoded reading into a Reading
func Unmarshall(req []byte) (*Reading, error) {
	// 00 01 02 03 04
	// T2 T1 HX V1 V2
	l := len(req)
	if l != 5 {
		log.Printf("[X] Expecting 5 bytes; got %d", l)
		return &Reading{}, fmt.Errorf("Expecting 5 bytes got %d", l)
	}
	// Temperature is stored little endian
	t := float64(int(binary.LittleEndian.Uint16(req[0:2]))) / 100.0
	h := float64(req[2])
	v := float64(int(binary.LittleEndian.Uint16(req[3:5]))) / 1000
	return &Reading{
		Temperature: t,
		Humidity:    h,
		Voltage:     v,
	}, nil
}
