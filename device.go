package main

import (
	"log/slog"
	"sync"
	"sync/atomic"
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
	// Use atomic for thread safety
	deviceResetNeeded int32 = 0

	// Track errors per device
	errorsPerDevice = make(map[string]int)
	errorsMutex     sync.Mutex
)

// RequestBLEDeviceReset marks the BLE device for reset
func RequestBLEDeviceReset() {
	slog.Warn("Explicitly requesting BLE device reset")
	atomic.StoreInt32(&deviceResetNeeded, 1)
}

// IsBLEDeviceResetRequested checks if a reset has been requested
func IsBLEDeviceResetRequested() bool {
	return atomic.LoadInt32(&deviceResetNeeded) == 1
}

// ClearBLEDeviceResetRequest clears the reset request
func ClearBLEDeviceResetRequest() {
	atomic.StoreInt32(&deviceResetNeeded, 0)
}

// IncrementErrors increments the error counter for a device
func IncrementErrors(deviceName string) int {
	errorsMutex.Lock()
	defer errorsMutex.Unlock()

	errorsPerDevice[deviceName]++
	current := errorsPerDevice[deviceName]

	// If we've accumulated too many errors, request a reset
	if current >= 3 {
		slog.Warn("Device has accumulated too many errors, requesting reset",
			"device", deviceName,
			"errorCount", current)
		RequestBLEDeviceReset()
	}

	return current
}

// ResetErrors resets the error counter for a device
func ResetErrors(deviceName string) {
	errorsMutex.Lock()
	defer errorsMutex.Unlock()

	errorsPerDevice[deviceName] = 0
}

// Device represents a BLE Device
type Device struct {
	Name   string
	Addr   string
	Client ble.Client
}

// Connect to a Device with retries
func (d *Device) Connect(host *linux.Device) (err error) {
	maxRetries := 3
	backoff := 1 * time.Second

	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			slog.Info("Retrying connection",
				"device", d.Name,
				"attempt", retry+1,
				"maxAttempts", maxRetries)
			time.Sleep(backoff)
			backoff *= 3 // Exponential backoff
		}

		// Use a shorter timeout for each attempt
		connectionTimeout := 30 * time.Second
		ctx := ble.WithSigHandler(context.WithTimeout(context.Background(), connectionTimeout))

		// Attempt to connect
		d.Client, err = host.Dial(ctx, ble.NewAddr(d.Addr))
		if err == nil {
			return nil // Successfully connected
		}

		slog.Info("Connection error",
			"device", d.Name,
			"error", err)
	}

	return err // Return the last error after all retries
}

// Disconnect from a Device
func (d *Device) Disconnect() error {
	return d.Client.CancelConnection()
}

// connectToDevice attempts to connect to the device and returns connection status
func (d *Device) connectToDevice() bool {
	slog.Info("Connecting to device", "device", d.Name)

	// Connect to device
	if err := d.Connect(bleDevice); err != nil {
		slog.Error("Failed to connect to device",
			"device", d.Name,
			"error", err)
		deviceErrorsCounter.WithLabelValues(d.Name).Inc()
		return false
	}

	return true
}

// calculateWaitTime determines the wait time before next reading based on success
func calculateWaitTime(success bool) time.Duration {
	if !success {
		// Use a shorter interval for retry after failure
		waitTime := time.Duration(*measurementInterval/2) * time.Second
		if waitTime < 10*time.Second {
			waitTime = 10 * time.Second // Minimum 10 seconds between retries
		}
		return waitTime
	}

	// Use normal interval
	return time.Duration(*measurementInterval) * time.Second
}

// handleDeviceOperation performs the main device operation (publishing and reading data)
func (d *Device) handleDeviceOperation() (bool, error) {
	// Use defer to ensure we always disconnect
	var disconnectErr error
	defer func() {
		// Disconnect to save battery
		slog.Info("Disconnecting to save battery", "device", d.Name)
		if err := d.Disconnect(); err != nil {
			disconnectErr = err
			slog.Error("Error disconnecting",
				"device", d.Name,
				"error", err)
		}
	}()

	// Write to handle to trigger notification
	slog.Info("Publishing", "device", d.Name)
	d.pub(characteristix[38], []byte{0x01, 0x00})

	// Subscribe to readings
	slog.Info("Subscribing", "device", d.Name)
	dataSuccess := d.readSensorData(characteristix[36])

	return dataSuccess, disconnectErr
}

// checkForResetNeeds checks if device reset is needed and requests it if so
func (d *Device) checkForResetNeeds(consecutiveFailures int, criticalError bool) bool {
	// Check if device reset is needed
	needsReset := false

	if consecutiveFailures >= 3 || criticalError {
		slog.Warn("Requesting BLE device reset due to persistent issues", "device", d.Name)
		RequestBLEDeviceReset()
		needsReset = true
	}

	return needsReset
}

// RegisterHandler registers a Temperature|Humidity handler
func RegisterHandler(d Device) {
	consecutiveFailures := 0
	maxConsecutiveFailures := 5
	waitTimeBetweenAttempts := time.Duration(*measurementInterval) * time.Second

	for {
		// Use the shared BLE device with mutex lock for synchronization
		slog.Info("Waiting for BLE device access", "device", d.Name)
		bleMutex.Lock()
		slog.Info("Acquired BLE device access", "device", d.Name)

		success := false
		criticalError := false

		// Step 1: Connect to device
		connected := d.connectToDevice()
		if !connected {
			consecutiveFailures++
		} else {
			// Step 2: Perform device operations if connected
			dataSuccess, err := d.handleDeviceOperation()

			if dataSuccess {
				// If data read was successful, reset failure counter
				success = true
				consecutiveFailures = 0
			} else {
				consecutiveFailures++
				// Connection was successful but data reading failed
				criticalError = err != nil
			}
		}

		// Step 3: Check if device reset is needed
		needsReset := d.checkForResetNeeds(consecutiveFailures, criticalError)

		// Step 4: Release BLE device access
		slog.Info("Releasing BLE device access", "device", d.Name)
		bleMutex.Unlock()

		// Step 5: Wait for reset if needed
		if needsReset && IsBLEDeviceResetRequested() {
			waitTime := 10 * time.Second
			slog.Info("Waiting for BLE device reset", "device", d.Name, "waitTime", waitTime)
			time.Sleep(waitTime)
		}

		// Step 6: Handle excessive failures
		if consecutiveFailures >= maxConsecutiveFailures {
			slog.Warn("Multiple consecutive failures",
				"device", d.Name,
				"failureCount", consecutiveFailures,
				"status", "device may be offline or have issues")

			// Reset counter to avoid log spam but continue trying
			consecutiveFailures = maxConsecutiveFailures / 2
		}

		// Step 7: Determine wait time before next reading
		waitTimeBetweenAttempts = calculateWaitTime(success)

		// Step 8: Wait before next reading
		slog.Info("Waiting before next reading",
			"device", d.Name,
			"waitTime", waitTimeBetweenAttempts)
		time.Sleep(waitTimeBetweenAttempts)
	}
}

func (d *Device) pub(c ble.UUID, b []byte) {
	slog.Info("Publishing",
		"device", d.Name,
		"uuid", c.String(),
		"value", b)
	if p, err := d.Client.DiscoverProfile(true); err == nil {
		if u := p.Find(ble.NewCharacteristic(c)); u != nil {
			c := u.(*ble.Characteristic)
			if err := d.Client.WriteCharacteristic(c, b, false); err != nil {
				slog.Error("Error writing characteristic", "error", err)
			}
		}
	}
}

// performWithRetry executes an operation with retries and tracks errors
func (d *Device) performWithRetry(operation string, maxRetries int,
	action func() error, onError func(error)) (success bool) {

	backoff := 1 * time.Second

	for retry := 0; retry < maxRetries; retry++ {
		if retry > 0 {
			slog.Info("Retrying operation",
				"device", d.Name,
				"operation", operation,
				"attempt", retry+1,
				"maxAttempts", maxRetries)
			time.Sleep(backoff)
			backoff *= 3 // Exponential backoff
		}

		err := action()
		if err == nil {
			return true // Success
		}

		// Call the onError handler if provided
		if onError != nil {
			onError(err)
		}
	}

	return false // Failed after all retries
}

// subscribeToCharacteristic subscribes to a characteristic with retries
func (d *Device) subscribeToCharacteristic(characteristic *ble.Characteristic, maxRetries int) (success bool, localErrors int) {
	slog.Info("Subscribing to characteristic",
		"device", d.Name,
		"handle", characteristic.Handle)

	subscribeAction := func() error {
		return d.Client.Subscribe(characteristic, false, handlerPublisher(d.Name))
	}

	onError := func(err error) {
		// Log and track subscribe errors
		slog.Error("Subscribe error",
			"device", d.Name,
			"handle", characteristic.Handle,
			"error", err)

		localErrors++
		totalErrors := IncrementErrors(d.Name)

		slog.Info("Tracked error",
			"device", d.Name,
			"localErrors", localErrors,
			"totalErrors", totalErrors,
			"errorType", "subscribe")
	}

	success = d.performWithRetry("subscription", maxRetries, subscribeAction, onError)

	if !success {
		slog.Error("Failed to subscribe after multiple attempts",
			"device", d.Name,
			"attempts", maxRetries)
	}

	return success, localErrors
}

// unsubscribeFromCharacteristic unsubscribes from a characteristic with retries
func (d *Device) unsubscribeFromCharacteristic(characteristic *ble.Characteristic, maxRetries int) (localErrors int) {
	slog.Info("Unsubscribing from characteristic",
		"device", d.Name,
		"handle", characteristic.Handle)

	unsubscribeAction := func() error {
		return d.Client.Unsubscribe(characteristic, false)
	}

	onError := func(err error) {
		// Log and track unsubscribe errors
		slog.Error("Unsubscribe error",
			"device", d.Name,
			"handle", characteristic.Handle,
			"error", err)

		localErrors++
		totalErrors := IncrementErrors(d.Name)

		slog.Info("Tracked error",
			"device", d.Name,
			"localErrors", localErrors,
			"totalErrors", totalErrors,
			"errorType", "unsubscribe")
	}

	success := d.performWithRetry("unsubscription", maxRetries, unsubscribeAction, onError)

	if !success {
		slog.Warn("Failed to unsubscribe cleanly, continuing anyway", "device", d.Name)
	}

	return localErrors
}

// discoverDeviceProfile discovers the device profile with retries
func (d *Device) discoverDeviceProfile(maxRetries int) (*ble.Profile, int) {
	slog.Info("Discovering device profile", "device", d.Name)

	var p *ble.Profile
	localErrors := 0

	discoverAction := func() error {
		var err error
		p, err = d.Client.DiscoverProfile(true)
		return err
	}

	onError := func(err error) {
		slog.Error("Discover profile error",
			"device", d.Name,
			"error", err)

		localErrors++
		totalErrors := IncrementErrors(d.Name)

		slog.Info("Tracked error",
			"device", d.Name,
			"localErrors", localErrors,
			"totalErrors", totalErrors,
			"errorType", "discover_profile")
	}

	success := d.performWithRetry("profile discovery", maxRetries, discoverAction, onError)

	if success {
		// Reset error counter on success (only on first try)
		if localErrors == 0 {
			ResetErrors(d.Name)
		}
		return p, localErrors
	}

	slog.Error("Failed to discover profile after multiple attempts",
		"device", d.Name,
		"attempts", maxRetries,
		"errors", localErrors)
	return nil, localErrors
}

func (d *Device) readSensorData(c ble.UUID) bool {
	slog.Info("Reading sensor data", "device", d.Name, "uuid", c.String())

	// Step 1: Discover device profile
	maxRetries := 3
	profile, errors := d.discoverDeviceProfile(maxRetries)
	if profile == nil {
		return false
	}

	// Step 2: Find the characteristic
	if u := profile.Find(ble.NewCharacteristic(c)); u != nil {
		characteristic := u.(*ble.Characteristic)

		// Check if this characteristic supports notifications and has CCCD
		if (characteristic.Property&ble.CharNotify) != 0 && characteristic.CCCD != nil {
			slog.Info("Registering Temperature|Humidity Handler",
				"device", d.Name,
				"handle", characteristic.Handle)

			// Step 3: Subscribe to notifications
			subscribed, subErrors := d.subscribeToCharacteristic(characteristic, maxRetries)
			errors += subErrors

			if !subscribed {
				return false
			}

			// Step 4: Wait for data
			time.Sleep(6 * time.Second)

			// Step 5: Unsubscribe
			errors += d.unsubscribeFromCharacteristic(characteristic, maxRetries)

			return true // Successfully read data
		}
	}

	return false // Failed to find characteristic
}
