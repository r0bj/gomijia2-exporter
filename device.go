package main

import (
	"log"
	"time"

	"github.com/currantlabs/ble"
	"github.com/currantlabs/ble/linux"
	"golang.org/x/net/context"
)

var (
	characteristix = map[uint8]ble.UUID{
		36: ble.MustParse("ebe0ccc1-7a0a-4b0c-8a1a-6ff2997da3a6"),
		38: ble.MustParse("00002902-0000-1000-8000-00805f9b34fb"),
	}
)

// Device represents a BLE Device
type Device struct {
	Name   string
	Addr   string
	Client ble.Client
}

// Connect to a Device
func (d *Device) Connect(host *linux.Device) (err error) {
	ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), 1*time.Minute))
	d.Client, err = host.Dial(ctx, ble.NewAddr(d.Addr))
	return err
}

// Disconnect from a Device
func (d *Device) Disconnect() error {
	return d.Client.CancelConnection()
}

// RegisterHandler registers a Temperature|Humidity handler
func RegisterHandler(d Device) {

	// Thanks to other developers
	// Write to handle `0x0038` with value `0x0100` is required to trigger notification of humidity|temperature
	log.Printf("[Device:RegisterHandler:%s] Publish", d.Name)
	d.pub(characteristix[38], []byte{0x01, 0x00})

	log.Printf("[Device:RegisterHandler:%s] Subscribe", d.Name)
	d.sub(characteristix[36])
}

func (d *Device) pub(c ble.UUID, b []byte) {
	log.Printf("[Device:pub:%s] Handler: %s (%x)", d.Name, c.String(), b)
	if p, err := d.Client.DiscoverProfile(true); err == nil {
		if u := p.Find(ble.NewCharacteristic(c)); u != nil {
			c := u.(*ble.Characteristic)
			if err := d.Client.WriteCharacteristic(c, b, false); err != nil {
				log.Print(err)
			}
		}
	}
}

func (d *Device) sub(c ble.UUID) {
	for {
		log.Printf("[Device:sub:%s] Handler: %s", d.Name, c.String())
		if p, err := d.Client.DiscoverProfile(true); err == nil {
			if u := p.Find(ble.NewCharacteristic(c)); u != nil {
				c := u.(*ble.Characteristic)
				// If this Characteristic suports notifications and there's a CCCD
				// Then subscribe to it
				if (c.Property&ble.CharNotify) != 0 && c.CCCD != nil {
					// Subscribe/Unsubscribe loop to slow down measurements to save the battery (default measurement rate is every ~6s)
					for {
						log.Printf("[Device:sub:%s] (%04x) Registering Temperature|Humidity Handler", d.Name, c.Handle)
						if err := d.Client.Subscribe(c, false, handlerPublisher(d.Name)); err != nil {
							log.Printf("[Device:sub:%s] (%04x) Subscribe Error: %v", d.Name, c.Handle, err)
						}
						time.Sleep(6 * time.Second)
						if err := d.Client.Unsubscribe(c, false); err != nil {
							log.Printf("[Device:sub:%s] (%04x) Unsubscribe Error: %v", d.Name, c.Handle, err)
						}
						time.Sleep(time.Duration(*measurementInterval) * time.Second)
					}
				}
			}
		} else {
			log.Printf("[Device:sub:%s] Handler: %s; Discover Profile Error: %v", d.Name, c.String(), err)
		}
		time.Sleep(time.Duration(*measurementInterval) * time.Second)
	}
}
