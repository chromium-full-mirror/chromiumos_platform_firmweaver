// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zephyr_dtsi

import (
	"strings"
	"testing"
)

func TestParseCSV_Valid(t *testing.T) {
	csvData := `PinNumber,NetName,Pull,Dir,IO,Enum,PinTypeOverride,InitialCondition,NetNameOverride,Voltage
A1,EC_WP_ODL,UP,IN,--,GPIO_WRITE_PROTECT_L,INPUT_L,,EC_WP_L,3.3V
A2,GPIO_TEST,N,OUT,TTL,,OUTPUT_H,HIGH,,1.8V
A3,EC_I2C_SENSOR_SCL,N,IO,OD,I2C_SENSOR_SCL,I2C_CLOCK,,,"3.3 V"
`

	configs, err := ParseCSV(strings.NewReader(csvData), "")
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	if len(configs) != 3 {
		t.Fatalf("Expected 3 configs, got %d", len(configs))
	}

	c1 := configs[0]
	if c1.PinNumber != "A1" || c1.NetName != "EC_WP_ODL" || c1.Pull != "UP" || c1.Dir != "IN" || c1.Enum != "GPIO_WRITE_PROTECT_L" || c1.PinTypeOverride != "INPUT_L" || c1.NetNameOverride != "EC_WP_L" || c1.Voltage != "3.3V" {
		t.Errorf("Unexpected config 1: %+v", c1)
	}

	c2 := configs[1]
	if c2.PinNumber != "A2" || c2.NetName != "GPIO_TEST" || c2.Pull != "N" || c2.Dir != "OUT" || c2.InitialCondition != "HIGH" {
		t.Errorf("Unexpected config 2: %+v", c2)
	}

	c3 := configs[2]
	expectedEnum := "I2C_SENSOR_SCL"
	if c3.Enum != expectedEnum {
		t.Errorf("Expected Enum %q, got %q", expectedEnum, c3.Enum)
	}
}

func TestParseCSV_MultiVariantColumn(t *testing.T) {
	csvData := `Signal Name,Subsystem,Description,DIR,Int,I/O Type,Internal PU,I/O Voltage,NPCX99 (NPCX),Type,Enum
GSC_EC_PWR_BTN_ODL,GSC,Power Button input,IN,both,--,Y,3.30 V,E7,INPUT_PU,GPIO_POWER_BUTTON_L
NC_UNCONNECTED,MISC,Unused pin,IN,no,--,N,1.80 V,F1,OTHER,
EC_EDP_BL_EN_OD,DISP,Backlight enable,OUT,no,OD,N,3.30 V,E10,OUTPUT_ODR,GPIO_ENABLE_BACKLIGHT
`

	configs, err := ParseCSV(strings.NewReader(csvData), "NPCX")
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	if len(configs) != 2 {
		t.Fatalf("Expected 2 configs (NC skipped), got %d", len(configs))
	}

	if configs[0].PinNumber != "E7" || configs[0].NetName != "GSC_EC_PWR_BTN_ODL" || configs[0].Pull != "UP" || configs[0].PinTypeOverride != "INPUT_L" {
		t.Errorf("Unexpected config 0: %+v", configs[0])
	}

	if configs[1].PinNumber != "E10" || configs[1].NetName != "EC_EDP_BL_EN_OD" || configs[1].InitialCondition != "HIGH" {
		t.Errorf("Unexpected config 1: %+v", configs[1])
	}
}

func TestParseCSV_MissingRequiredColumn(t *testing.T) {
	csvData := `PinName,Pull,Dir,Enum
A1,UP,IN,GPIO_TEST
`

	_, err := ParseCSV(strings.NewReader(csvData), "")
	if err == nil {
		t.Error("Expected error for missing NetName column, got nil")
	}
}

func TestParseCSV_CorruptAndEmptyInputs(t *testing.T) {
	// Empty CSV Reader
	_, err := ParseCSV(strings.NewReader(""), "")
	if err == nil || !strings.Contains(err.Error(), "failed to read header") {
		t.Errorf("Expected header read error for empty CSV, got %v", err)
	}

	// Broken CSV format (unmatched quote)
	corruptCSV := "PinNumber,NetName\nA1,\"UNCLOSED_QUOTE\n"
	_, err = ParseCSV(strings.NewReader(corruptCSV), "")
	if err == nil || !strings.Contains(err.Error(), "failed to read row") {
		t.Errorf("Expected row read error for corrupt CSV, got %v", err)
	}

	// Missing primary pin header with explicit pinCol
	missingPinCSV := "HeaderNumber,NetName\nA1,EC_FOO\n"
	_, err = ParseCSV(strings.NewReader(missingPinCSV), "SpecifiedCol")
	if err == nil || !strings.Contains(err.Error(), "missing required column for Pin Number") {
		t.Errorf("Expected missing column error, got %v", err)
	}
}

func TestParseCSV_SignalFilteringExclusions(t *testing.T) {
	csvData := `PinNumber,NetName
A1,NC
A2,NC (TP)
A3,NC(TP)
A4,NC_TP
A5,NC_SPARE
A6,NC 123
A7,OPEN
A8,(OPEN)
A9,FLOATING
B1,"(OPEN, FLOATING)"
B2,N/A
B3,NA
B4,
B5,VALID_SIGNAL
`
	configs, err := ParseCSV(strings.NewReader(csvData), "")
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("Expected only 1 valid config (VALID_SIGNAL), got %d (configs: %+v)", len(configs), configs)
	}
	if configs[0].NetName != "VALID_SIGNAL" || configs[0].PinNumber != "B5" {
		t.Errorf("Unexpected remaining config: %+v", configs[0])
	}
}

func TestParseCSV_FallbackPinNumberResolution(t *testing.T) {
	csvData := `NPCX993 (Main),NPCX993 (Board),NetName,IO,Enum
N/A,B3,EC_I2C_SCL,I2C,I2C_PORT_0
N/A,B4,EC_GPIO_IN,IN,
--,,EC_PWM_FAN,PWM,
`
	configs, err := ParseCSV(strings.NewReader(csvData), "Main")
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}

	// Primary N/A for peripheral I2C should fall back to Secondary B3
	if len(configs) != 1 {
		t.Fatalf("Expected 1 valid config via fallback, got %d", len(configs))
	}
	if configs[0].PinNumber != "B3" || configs[0].NetName != "EC_I2C_SCL" {
		t.Errorf("Fallback failed, got config: %+v", configs[0])
	}

	// Test fallback when primary header has NO parentheses
	csvNoParens := `PinNumber,AltPin,NetName,IO,Enum
N/A,C2,EC_I2C_SDA,I2C,I2C_PORT_0
`
	configsNoParens, err := ParseCSV(strings.NewReader(csvNoParens), "AltPin")
	if err != nil {
		t.Fatalf("ParseCSV (no parens) failed: %v", err)
	}
	if len(configsNoParens) != 1 || configsNoParens[0].PinNumber != "C2" {
		t.Errorf("Fallback without parens failed, got: %+v", configsNoParens)
	}
}

func TestParseCSV_TypeAndConditionNormalizations(t *testing.T) {
	csvData := `PinNumber,NetName,Dir,PinTypeOverride,InitialCondition
A1,EC_WP_ODL,IN,INPUT_PU,
A2,EC_SENSOR_LOW,IN,INPUT,
A3,EC_RST_L,OUT,OUTPUT_L,
A4,EC_POWER_ON,OUT,OUTPUT_H,
A5,EC_PCH_WAKE_ODL,OUT,,
`
	configs, err := ParseCSV(strings.NewReader(csvData), "")
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}
	if len(configs) != 5 {
		t.Fatalf("Expected 5 configs, got %d", len(configs))
	}

	// A1: INPUT_PU + _ODL -> INPUT_L, Pull UP
	if configs[0].PinTypeOverride != "INPUT_L" || configs[0].Pull != "UP" {
		t.Errorf("Config A1 mismatch: %+v", configs[0])
	}
	// A2: INPUT + _LOW -> INPUT_L
	if configs[1].PinTypeOverride != "INPUT_L" {
		t.Errorf("Config A2 mismatch: %+v", configs[1])
	}
	// A3: OUTPUT_L -> InitialCondition LOW
	if configs[2].InitialCondition != "LOW" {
		t.Errorf("Config A3 initial condition expected LOW, got %q", configs[2].InitialCondition)
	}
	// A4: OUTPUT_H -> InitialCondition HIGH
	if configs[3].InitialCondition != "HIGH" {
		t.Errorf("Config A4 initial condition expected HIGH, got %q", configs[3].InitialCondition)
	}
	// A5: EC_PCH_WAKE_ODL special case -> InitialCondition HIGH
	if configs[4].InitialCondition != "HIGH" {
		t.Errorf("Config A5 initial condition expected HIGH, got %q", configs[4].InitialCondition)
	}
}

func TestParseCSV_HeaderNormalization(t *testing.T) {
	csvData := `"NPCX9
RVP",NetName
A1,EC_GPIO_A1
`
	configs, err := ParseCSV(strings.NewReader(csvData), "NPCX9 RVP")
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}
	if len(configs) != 1 {
		t.Fatalf("Expected 1 config, got %d", len(configs))
	}
	if configs[0].PinNumber != "A1" || configs[0].NetName != "EC_GPIO_A1" {
		t.Errorf("Unexpected config: %+v", configs[0])
	}
}

func TestParseRecords(t *testing.T) {
	if _, err := ParseRecords(nil, "NPCX"); err == nil {
		t.Errorf("expected error for nil records")
	}
	if _, err := ParseRecords([][]string{}, "NPCX"); err == nil {
		t.Errorf("expected error for empty records")
	}

	records := [][]string{
		{"Signal Name", "NPCX99 (NPCX)", "DIR"},
		{"EC_WP_ODL", "A1", "IN"},
	}
	configs, err := ParseRecords(records, "NPCX")
	if err != nil {
		t.Fatalf("ParseRecords failed: %v", err)
	}
	if len(configs) != 1 || configs[0].PinNumber != "A1" || configs[0].NetName != "EC_WP_ODL" {
		t.Errorf("unexpected configs from ParseRecords: %+v", configs)
	}
}

func TestParseRecords_FormulaErrors(t *testing.T) {
	records := [][]string{
		{"Signal Name", "DIR", "IO Type", "Enum", "NPCX99 (NPCX)", "NPCX99 (Board)"},
		{"EC_I2C_SCL", "IO", "I2C", "I2C_PORT_0", "#REF!", "B3"},
		{"EC_BROKEN_PIN", "IN", "", "", "#VALUE!", "#N/A"},
	}
	configs, err := ParseRecords(records, "NPCX")
	if err != nil {
		t.Fatalf("ParseRecords failed: %v", err)
	}
	if len(configs) != 1 || configs[0].PinNumber != "B3" {
		t.Fatalf("Expected fallback pin B3, got: %+v", configs)
	}
}

func TestParseCSV_PackageHeaderAutoDetection(t *testing.T) {
	csvData := `"128-pin VFBGA",IT82002 Pin Name,GPIO,Signal Name,Dir,Type
M5,PWM0/GPA0,GPA0,EC_VOLDN_BTN_ODL,,UNUSED_IN
M7,PWM6/SSCK/GPA6,GPA6,NC,,UNUSED_IN
C1,SMDAT2ALT/GPF7,GPF7,NC,,
L1,WRST#,WRST#,EC_ERST_L,IN,INPUT_L
`
	configs, err := ParseCSV(strings.NewReader(csvData), "")
	if err != nil {
		t.Fatalf("ParseCSV failed: %v", err)
	}
	if len(configs) != 3 {
		t.Fatalf("Expected 3 configs (NC with UNUSED_IN kept, plain NC skipped), got %d: %+v", len(configs), configs)
	}
	if configs[0].PinNumber != "M5" || configs[0].PinName != "PWM0/GPA0" || configs[0].PinTypeOverride != "UNUSED_IN" {
		t.Errorf("Unexpected config[0]: %+v", configs[0])
	}
	if configs[1].PinNumber != "M7" || configs[1].NetName != "NC" || configs[1].PinTypeOverride != "UNUSED_IN" {
		t.Errorf("Unexpected config[1]: %+v", configs[1])
	}
	if configs[2].PinNumber != "L1" || configs[2].PinName != "WRST#" || configs[2].NetName != "EC_ERST_L" {
		t.Errorf("Unexpected config[2]: %+v", configs[2])
	}

	// Ensure "IT82002 Pin Name" alone is not mistaken for a PinNumber column
	noPinNumCSV := "IT82002 Pin Name,Signal Name\nPWM0/GPA0,EC_FOO\n"
	if _, err := ParseCSV(strings.NewReader(noPinNumCSV), ""); err == nil || !strings.Contains(err.Error(), "missing required column for Pin Number") {
		t.Errorf("Expected missing Pin Number column error when only Pin Name is present, got %v", err)
	}
}
