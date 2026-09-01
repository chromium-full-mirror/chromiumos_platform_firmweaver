// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zephyr_dtsi

import (
	"os"
	"path/filepath"
	"testing"
)

func getChipOrFatal(t *testing.T, name string) *Chip {
	c, ok := GetChip(name)
	if !ok {
		t.Fatalf("Chip %s not found", name)
	}
	return c
}

func TestNPCX99_ResolveGPIO(t *testing.T) {
	chip := getChipOrFatal(t, "npcx99")

	tests := []struct {
		pinNum       string
		expectedPort string
		expectedPin  int
		expectErr    bool
	}{
		{"A2", "3", 1, false},  // KSI0/GPIO31/TRACEDATA3/GP_MOSI
		{"B10", "0", 6, false}, // KSO11/GPIO06
		{"D10", "c", 6, false}, // GPIOC6/SMI#
		{"D1", "", 0, true},    // AVCC (not a GPIO)
		{"Z99", "", 0, true},   // Pin not found
	}

	for _, tt := range tests {
		port, pin, err := chip.ResolveGPIO(tt.pinNum)
		if (err != nil) != tt.expectErr {
			t.Errorf("ResolveGPIO(%s) error status mismatch: expected err=%t, got err=%v", tt.pinNum, tt.expectErr, err)
			continue
		}
		if !tt.expectErr {
			if port != tt.expectedPort || pin != tt.expectedPin {
				t.Errorf("ResolveGPIO(%s) mismatch: expected port=%s pin=%d, got port=%s pin=%d", tt.pinNum, tt.expectedPort, tt.expectedPin, port, pin)
			}
		}
	}
}

func TestITE82002_ResolveGPIO(t *testing.T) {
	chip := getChipOrFatal(t, "it82002")

	tests := []struct {
		pinNum       string
		expectedPort string
		expectedPin  int
		expectErr    bool
	}{
		{"A10", "f", 2, false},    // PS2CLK1/DTR0#/SMINT8/GPF2
		{"M6", "a", 2, false},     // PWM2/GPA2
		{"N1", "d", 0, false},     // SMDAT2/RI1#/GPD0
		{"A3", "b", 1, false},     // SOUT0/GPB1
		{"A4", "b", 0, false},     // SIN0/GPB0
		{"M8", "ksol", 0, false},  // KSO0/PD0
		{"M13", "ksoh", 4, false}, // KSO12/SLCT
		{"Z99", "", 0, true},      // Non-existent pin, should fail ResolveGPIO
	}

	for _, tt := range tests {
		port, pin, err := chip.ResolveGPIO(tt.pinNum)
		if (err != nil) != tt.expectErr {
			t.Errorf("ResolveGPIO(%s) error status mismatch: expected err=%t, got err=%v", tt.pinNum, tt.expectErr, err)
			continue
		}
		if !tt.expectErr {
			if port != tt.expectedPort || pin != tt.expectedPin {
				t.Errorf("ResolveGPIO(%s) mismatch: expected port=%s pin=%d, got port=%s pin=%d", tt.pinNum, tt.expectedPort, tt.expectedPin, port, pin)
			}
		}
	}
}

func TestNPCX99_ResolveADC(t *testing.T) {
	chip := getChipOrFatal(t, "npcx99")
	tests := []struct {
		pinNum       string
		expectedChan int
		expectErr    bool
	}{
		{"C2", 4, false}, // GPIO41/ADC4
		{"D3", 3, false}, // GPIO42/ADC3/RI#
		{"D10", 0, true}, // GPIOC6/SMI# (no ADC)
		{"Z99", 0, true}, // Pin not found
	}

	for _, tt := range tests {
		ch, err := chip.ResolveADC(tt.pinNum)
		if (err != nil) != tt.expectErr {
			t.Errorf("ResolveADC(%s) error status: expected err=%t, got %v", tt.pinNum, tt.expectErr, err)
			continue
		}
		if !tt.expectErr && ch != tt.expectedChan {
			t.Errorf("ResolveADC(%s) expected %d, got %d", tt.pinNum, tt.expectedChan, ch)
		}
	}
}

func TestITE82002_ResolvePWM(t *testing.T) {
	chip := getChipOrFatal(t, "it82002")
	tests := []struct {
		pinNum       string
		expectedChan int
		expectErr    bool
	}{
		{"M6", 2, false}, // PWM2/GPA2
		{"M7", 6, false}, // PWM6/SSCK/GPA6
		{"M8", 0, true},  // KSO0/PD0 (no PWM)
		{"Z99", 0, true}, // Pin not found
	}

	for _, tt := range tests {
		ch, err := chip.ResolvePWM(tt.pinNum)
		if (err != nil) != tt.expectErr {
			t.Errorf("ResolvePWM(%s) error status: expected err=%t, got %v", tt.pinNum, tt.expectErr, err)
			continue
		}
		if !tt.expectErr && ch != tt.expectedChan {
			t.Errorf("ResolvePWM(%s) expected %d, got %d", tt.pinNum, tt.expectedChan, ch)
		}
	}
}

func TestChips_ResolveI2C(t *testing.T) {
	npcx := getChipOrFatal(t, "npcx99")
	ite := getChipOrFatal(t, "it82002")
	it81202 := getChipOrFatal(t, "it81202")
	it81302 := getChipOrFatal(t, "it81302")

	tests := []struct {
		chip         *Chip
		pinNum       string
		expectedChan string
		expectedScl  bool
		expectErr    bool
	}{
		// NPCX
		{npcx, "B12", "0", false, false}, // GPIOB4/I2C0_SDA0
		{npcx, "C12", "0", true, false},  // GPIOB5/I2C0_SCL0
		{npcx, "A2", "", false, true},    // KSI0/GPIO31/TRACEDATA3/GP_MOSI (no I2C)
		{npcx, "Z99", "", false, true},   // Pin not found

		// ITE
		{ite, "A5", "0", true, false},     // SMCLK0/GPB3
		{ite, "A2", "0", false, false},    // SMDAT0/GPB4
		{ite, "B1", "2ALT", true, false},  // SMCLK2ALT/PECI/GPF6
		{ite, "C1", "2ALT", false, false}, // SMDAT2ALT/GPF7
		{ite, "M8", "", false, true},      // KSO0/PD0 (no I2C)

		// IT81202
		{it81202, "M2", "2ALT", true, false},  // SMCLK2ALT
		{it81202, "C1", "2ALT", false, false}, // SMDAT2ALT (prioritized over SMDAT2)
		{it81202, "B1", "2", true, false},     // SMCLK2

		// IT81302
		{it81302, "N1", "2ALT", true, false},   // SMCLK2ALT
		{it81302, "N15", "2ALT", false, false}, // SMDAT2ALT
	}

	for _, tt := range tests {
		ch, scl, err := tt.chip.ResolveI2C(tt.pinNum)
		if (err != nil) != tt.expectErr {
			t.Errorf("[%s] ResolveI2C(%s) error status: expected err=%t, got %v", tt.chip.Name, tt.pinNum, tt.expectErr, err)
			continue
		}
		if !tt.expectErr {
			if ch != tt.expectedChan || scl != tt.expectedScl {
				t.Errorf("[%s] ResolveI2C(%s) mismatch: expected chan=%s scl=%t, got chan=%s scl=%t", tt.chip.Name, tt.pinNum, tt.expectedChan, tt.expectedScl, ch, scl)
			}
		}
	}
}

func TestChips_EmbeddedDataAndRegistration(t *testing.T) {
	expectedChips := []string{"npcx99", "it81202", "it81302", "it82002", "it82202"}
	for _, name := range expectedChips {
		chip, ok := GetChip(name)
		if !ok || chip == nil {
			t.Errorf("Embedded chip %q missing from registered chips", name)
			continue
		}
		if len(chip.PinMap) == 0 {
			t.Errorf("Embedded chip %q has empty PinMap", name)
		}
	}
}

func TestITE_KSO_ResolveGPIO(t *testing.T) {
	chip := &Chip{
		Name:   "it81302",
		Vendor: "ite",
		PinMap: map[string]string{
			"P1": "KSO3",
			"P2": "KSO12",
			"P3": "KSO_INVALID",
			"P4": "KSO16",
		},
	}

	port, pin, err := chip.ResolveGPIO("P1")
	if err != nil || port != "ksol" || pin != 3 {
		t.Errorf("ResolveGPIO(P1 KSO3) expected port=ksol pin=3, got port=%s pin=%d err=%v", port, pin, err)
	}

	port, pin, err = chip.ResolveGPIO("P2")
	if err != nil || port != "ksoh" || pin != 4 {
		t.Errorf("ResolveGPIO(P2 KSO12) expected port=ksoh pin=4 (12-8), got port=%s pin=%d err=%v", port, pin, err)
	}

	_, _, err = chip.ResolveGPIO("P3")
	if err == nil {
		t.Error("Expected error for KSO_INVALID, got nil")
	}

	_, _, err = chip.ResolveGPIO("P4")
	if err == nil {
		t.Error("Expected error for KSO16, got nil")
	}
}

func TestITE82202_FullResolution(t *testing.T) {
	chip := getChipOrFatal(t, "it82202")

	// ResolveGPIO
	port, pin, err := chip.ResolveGPIO("M6") // PWM2/GPA2
	if err != nil || port != "a" || pin != 2 {
		t.Errorf("it82202 ResolveGPIO(M6) failed: port=%s pin=%d err=%v", port, pin, err)
	}

	// ResolvePWM
	pwmChan, err := chip.ResolvePWM("M6")
	if err != nil || pwmChan != 2 {
		t.Errorf("it82202 ResolvePWM(M6) failed: pwm=%d err=%v", pwmChan, err)
	}

	// ResolveI2C
	i2cChan, isScl, err := chip.ResolveI2C("B4") // SMCLK0/GPB3
	if err != nil || i2cChan != "0" || !isScl {
		t.Errorf("it82202 ResolveI2C(B4) failed: chan=%s isScl=%t err=%v", i2cChan, isScl, err)
	}
}

func TestRegisterChipData_InvalidJSON(t *testing.T) {
	err := RegisterChipData([]byte(`{invalid json`))
	if err == nil {
		t.Error("Expected error for invalid JSON, got nil")
	}
}

func TestLoadExternalChipDir(t *testing.T) {
	// Test non-existent directory
	err := LoadExternalChipDir("non_existent_directory_xyz")
	if err == nil {
		t.Error("Expected error for non-existent directory, got nil")
	}

	// Test valid directory with a valid chip and an invalid chip
	tmpDir, err := os.MkdirTemp("", "test_chips")
	if err != nil {
		t.Fatalf("Failed to create temp dir: %v", err)
	}
	defer os.RemoveAll(tmpDir)

	// Write a valid chip JSON
	validChipJSON := `{
		"name": "temp-mock-chip",
		"vendor": "mock",
		"pin_map": {
			"A1": "GPIOA1"
		}
	}`
	err = os.WriteFile(filepath.Join(tmpDir, "temp_chip.json"), []byte(validChipJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write valid chip file: %v", err)
	}

	// Write an invalid chip JSON
	invalidChipJSON := `{invalid json`
	err = os.WriteFile(filepath.Join(tmpDir, "invalid_chip.json"), []byte(invalidChipJSON), 0644)
	if err != nil {
		t.Fatalf("Failed to write invalid chip file: %v", err)
	}

	// This should fail because of the invalid chip JSON
	err = LoadExternalChipDir(tmpDir)
	if err == nil {
		t.Error("Expected error when loading directory with invalid JSON file, got nil")
	}

	// Remove the invalid chip and try again
	err = os.Remove(filepath.Join(tmpDir, "invalid_chip.json"))
	if err != nil {
		t.Fatalf("Failed to remove invalid chip file: %v", err)
	}

	err = LoadExternalChipDir(tmpDir)
	if err != nil {
		t.Errorf("Expected success when loading valid chip directory, got error: %v", err)
	}

	// Verify the temp-mock-chip was registered
	if _, ok := GetChip("temp-mock-chip"); !ok {
		t.Error("temp-mock-chip was not registered")
	}
}

func TestChips_ResolveTACH(t *testing.T) {
	npcx := getChipOrFatal(t, "npcx99")
	ite := getChipOrFatal(t, "it82002")

	tests := []struct {
		chip         *Chip
		pinNum       string
		expectedChan int
		expectedPort string
		expectErr    bool
	}{
		// NPCX
		{npcx, "E5", 1, "A", false}, // GPIO40/TA1
		{npcx, "A2", 0, "", true},   // KSI0/GPIO31/TRACEDATA3/GP_MOSI (no TACH)
		{npcx, "Z99", 0, "", true},  // Pin not found

		// ITE
		{ite, "M11", 0, "A", false}, // TACH0A/GPD6
		{ite, "A10", 0, "", true},   // PS2CLK1/DTR0#/SMINT8/GPF2 (no TACH)
	}

	for _, tt := range tests {
		ch, port, err := tt.chip.ResolveTACH(tt.pinNum)
		if (err != nil) != tt.expectErr {
			t.Errorf("%s ResolveTACH(%s) error status: expected err=%t, got %v", tt.chip.Name, tt.pinNum, tt.expectErr, err)
			continue
		}
		if !tt.expectErr {
			if ch != tt.expectedChan || port != tt.expectedPort {
				t.Errorf("%s ResolveTACH(%s) expected chan=%d port=%s, got chan=%d port=%s", tt.chip.Name, tt.pinNum, tt.expectedChan, tt.expectedPort, ch, port)
			}
		}
	}
}
