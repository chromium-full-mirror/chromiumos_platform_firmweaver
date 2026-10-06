// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zephyr_dtsi

import (
	"bytes"
	"os"
	"strings"
	"testing"
)

func TestGenerateDTSI_Mock(t *testing.T) {
	mockPinMap := map[string]string{
		"A1": "ADC0",
		"B2": "GPIOB4/I2C0_SDA0",
		"B3": "GPIOB5/I2C0_SCL0",
		"C3": "GPIOC3/PWM0",
		"D4": "GPIOD4",
		"E5": "GPIOE5",
		"F5": "GPIOF5/TB1",
		"G1": "KSI0/GPIO31",
		"G2": "KSO01/GPIO21",
	}

	chip := &Chip{
		Name:   "npcx-mock",
		Vendor: "npcx",
		PinMap: mockPinMap,
	}

	configs := []PinConfig{
		{
			PinNumber: "A1",
			PinName:   "ADC_0",
			NetName:   "EC_ADC0",
			IO:        "ADC",
			Enum:      "ADC_EVB_CH_0",
		},
		{
			PinNumber:       "B3",
			PinName:         "I2C0_SCL",
			NetName:         "EC_I2C_SCL",
			Enum:            "I2C_PORT_0",
			IO:              "OD",
			PinTypeOverride: "I2C_CLOCK",
		},
		{
			PinNumber:       "B2",
			PinName:         "I2C0_SDA",
			NetName:         "EC_I2C_SDA",
			IO:              "OD",
			PinTypeOverride: "I2C_DATA",
		},
		{
			PinNumber: "C3",
			PinName:   "PWM_0",
			NetName:   "EC_PWM0",
			IO:        "PWM",
		},
		{
			PinNumber: "D4",
			PinName:   "GPIO_D4",
			NetName:   "EC_GPIO_D4",
			Dir:       "OUT",
			IO:        "OD",
		},
		{
			PinNumber:       "E5",
			PinName:         "GPIO_E5",
			NetName:         "EC_GPIO_E5_L",
			Dir:             "IN",
			Pull:            "UP",
			PinTypeOverride: "INPUT_L",
		},
		{
			PinNumber: "F5",
			PinName:   "TACH_1",
			NetName:   "EC_FAN_TACH",
		},
		{
			PinNumber:       "G1",
			PinName:         "KSI0",
			NetName:         "EC_KSI0",
			PinTypeOverride: "KEYBOARD",
		},
		{
			PinNumber:       "G2",
			PinName:         "KSO01",
			NetName:         "EC_KSO01",
			PinTypeOverride: "KEYBOARD",
		},
	}

	var out bytes.Buffer
	err := GenerateDTSI(&out, configs, chip)
	if err != nil {
		t.Fatalf("GenerateDTSI failed: %v", err)
	}

	got := out.String()
	requiredSnippets := []string{
		"named-adc-channels",
		"named-gpios",
		"named-i2c-ports",
		"&adc0",
		"&pwm0",
		"&tach1",
		"&kbd",
	}

	for _, snip := range requiredSnippets {
		if !strings.Contains(got, snip) {
			t.Errorf("GenerateDTSI output missing required snippet %q", snip)
		}
	}
}

func TestGenerateDTSI_ITE_Mock(t *testing.T) {
	mockPinMap := map[string]string{
		"A1": "ADC0/GPA0",
		"B3": "SMCLK0/GPB3",
		"B4": "SMDAT0/GPB4",
		"C3": "PWM0/GPC3",
		"D4": "GPIOD4",
		"D6": "TACH0A/GPD6",
		"G1": "KSI0/GPG1",
		"G2": "KSO0/GPG2",
	}

	chip := &Chip{
		Name:   "ite-mock",
		Vendor: "ite",
		PinMap: mockPinMap,
	}

	configs := []PinConfig{
		{
			PinNumber: "A1",
			PinName:   "ADC_0",
			NetName:   "EC_ADC0",
			IO:        "ADC",
			Enum:      "ADC_EVB_CH_0",
		},
		{
			PinNumber:       "B3",
			PinName:         "I2C0_SCL",
			NetName:         "EC_I2C",
			Enum:            "I2C_PORT_0",
			IO:              "OD",
			PinTypeOverride: "I2C_CLOCK",
		},
		{
			PinNumber:       "B4",
			PinName:         "I2C0_SDA",
			NetName:         "EC_I2C",
			IO:              "OD",
			PinTypeOverride: "I2C_DATA",
		},
		{
			PinNumber: "C3",
			PinName:   "PWM_0",
			NetName:   "EC_PWM0",
			IO:        "PWM",
		},
		{
			PinNumber:       "D6",
			PinName:         "TACH_0",
			NetName:         "EC_FAN_TACH",
			PinTypeOverride: "TACH",
		},
		{
			PinNumber:       "G1",
			PinName:         "KSI0",
			NetName:         "EC_KSI0",
			PinTypeOverride: "KEYBOARD",
		},
		{
			PinNumber:       "G2",
			PinName:         "KSO0",
			NetName:         "EC_KSO0",
			PinTypeOverride: "KEYBOARD",
		},
	}

	var out bytes.Buffer
	err := GenerateDTSI(&out, configs, chip)
	if err != nil {
		t.Fatalf("GenerateDTSI failed: %v", err)
	}

	got := out.String()
	requiredSnippets := []string{
		"named-adc-channels",
		"named-i2c-ports",
		"&adc0",
		"&i2c0",
		"&pwm0",
		"&tach0",
		"&kbd",
	}

	for _, snip := range requiredSnippets {
		if !strings.Contains(got, snip) {
			t.Errorf("ITE DTSI output missing snippet %q", snip)
		}
	}
}

func TestGenerateDTSI_SubsystemErrorPaths(t *testing.T) {
	chip := &Chip{
		Name:   "npcx-mock",
		Vendor: "npcx",
		PinMap: map[string]string{"D4": "GPIOD4"}, // D4 supports no peripheral
	}

	// Test ADC failure on non-ADC pin
	adcConfig := []PinConfig{{PinNumber: "D4", NetName: "ADC_FAIL", IO: "ADC"}}
	var buf bytes.Buffer
	if err := GenerateDTSI(&buf, adcConfig, chip); err == nil || !strings.Contains(err.Error(), "failed to resolve ADC") {
		t.Errorf("Expected ADC resolution error, got %v", err)
	}

	// Test PWM failure on non-PWM pin
	pwmConfig := []PinConfig{{PinNumber: "D4", NetName: "PWM_FAIL", IO: "PWM"}}
	buf.Reset()
	if err := GenerateDTSI(&buf, pwmConfig, chip); err == nil || !strings.Contains(err.Error(), "failed to resolve PWM") {
		t.Errorf("Expected PWM resolution error, got %v", err)
	}

	// Test I2C failure on non-I2C pin
	i2cConfig := []PinConfig{{PinNumber: "D4", NetName: "I2C_FAIL", IO: "OD", PinTypeOverride: "I2C_CLOCK"}}
	buf.Reset()
	if err := GenerateDTSI(&buf, i2cConfig, chip); err == nil || !strings.Contains(err.Error(), "failed to resolve I2C") {
		t.Errorf("Expected I2C resolution error, got %v", err)
	}
}

func TestGenerateDTSI_DuplicatePinsAndExclusions(t *testing.T) {
	chip := &Chip{
		Name:   "npcx-mock",
		Vendor: "npcx",
		PinMap: map[string]string{
			"D4": "GPIOD4",
			"D5": "GPIOD5",
			"D6": "GPIOD6",
		},
	}

	configs := []PinConfig{
		{PinNumber: "D4", NetName: "EC_GPIO_D4", Dir: "OUT"},
		{PinNumber: "D4", NetName: "EC_DUPLICATE_D4", Dir: "OUT"},                      // Should be ignored
		{PinNumber: "D5", NetName: "ESPI_CLK", Dir: "IN"},                              // Excluded prefix
		{PinNumber: "D6", NetName: "EC_GPIO_D6", Dir: "OUT", PinTypeOverride: "OTHER"}, // Excluded type
	}

	var buf bytes.Buffer
	if err := GenerateDTSI(&buf, configs, chip); err != nil {
		t.Fatalf("GenerateDTSI failed: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "ec_duplicate_d4") {
		t.Error("Duplicate pin D4 was not deduplicated")
	}
	if strings.Contains(out, "espi_clk") {
		t.Error("ESPI_CLK signal was not excluded")
	}
	if strings.Contains(out, "ec_gpio_d6") {
		t.Error("PinTypeOverride OTHER was not excluded")
	}
	if !strings.Contains(out, "gpio_ec_gpio_d4: ec_gpio_d4") {
		t.Error("Primary pin D4 missing from output")
	}
}

func TestGenerateDTSI_AllGPIOFlags(t *testing.T) {
	chip := &Chip{
		Name:   "npcx-mock",
		Vendor: "npcx",
		PinMap: map[string]string{
			"A1": "GPIOA1",
		},
	}

	configs := []PinConfig{
		{
			PinNumber:        "A1",
			NetName:          "EC_FLAG_TEST",
			Dir:              "OUT",
			PinTypeOverride:  "ODR_ACTIVE_L",
			Voltage:          "1.8V",
			InitialCondition: "HIGH",
			Pull:             "DOWN",
		},
	}

	var buf bytes.Buffer
	if err := GenerateDTSI(&buf, configs, chip); err != nil {
		t.Fatalf("GenerateDTSI failed: %v", err)
	}

	expectedFlags := "(GPIO_OUTPUT | GPIO_OPEN_DRAIN | GPIO_ACTIVE_LOW | GPIO_PULL_DOWN | GPIO_OUTPUT_INIT_HIGH | GPIO_VOLTAGE_1P8)"
	if !strings.Contains(buf.String(), expectedFlags) {
		t.Errorf("Output missing expected flags %q.\nGot:\n%s", expectedFlags, buf.String())
	}
}

func TestIntegration_ExampleEcPinout_With_RealChip(t *testing.T) {
	csvFile := "test/ExampleEcPinout.csv"
	data, err := os.ReadFile(csvFile)
	if err != nil {
		t.Skipf("Skipping integration test: %s not found", csvFile)
	}

	t.Run("NPCX99", func(t *testing.T) {
		chip, ok := GetChip("npcx99")
		if !ok {
			t.Fatal("npcx99 chip definition missing")
		}

		configs, err := ParseCSV(bytes.NewReader(data), "NPCX99 (NPCX)")
		if err != nil {
			t.Fatalf("ParseCSV failed: %v", err)
		}

		var buf bytes.Buffer
		if err := GenerateDTSI(&buf, configs, chip); err != nil {
			t.Fatalf("GenerateDTSI failed: %v", err)
		}

		expected, err := os.ReadFile("test/generated_npcx.dtsi")
		if err != nil {
			t.Fatalf("Failed to read test/generated_npcx.dtsi: %v", err)
		}
		if buf.String() != string(expected) {
			t.Errorf("Generated NPCX99 DTSI differs from test/generated_npcx.dtsi.\nGot:\n%s\nWant:\n%s", buf.String(), string(expected))
		}
	})

	t.Run("IT82002", func(t *testing.T) {
		chip, ok := GetChip("it82002")
		if !ok {
			t.Fatal("it82002 chip definition missing")
		}

		configs, err := ParseCSV(bytes.NewReader(data), "IT82002 (ITE)")
		if err != nil {
			t.Fatalf("ParseCSV failed: %v", err)
		}

		var buf bytes.Buffer
		if err := GenerateDTSI(&buf, configs, chip); err != nil {
			t.Fatalf("GenerateDTSI failed: %v", err)
		}

		expected, err := os.ReadFile("test/generated_ite.dtsi")
		if err != nil {
			t.Fatalf("Failed to read test/generated_ite.dtsi: %v", err)
		}
		if buf.String() != string(expected) {
			t.Errorf("Generated IT82002 DTSI differs from test/generated_ite.dtsi.\nGot:\n%s\nWant:\n%s", buf.String(), string(expected))
		}
	})
}

func TestGenerateDTSI_IsGPIOBranches(t *testing.T) {
	chip := &Chip{
		Name:   "npcx-mock",
		Vendor: "npcx",
		PinMap: map[string]string{
			"D4": "GPIOD4",
			"D5": "GPIOD5",
		},
	}

	configs := []PinConfig{
		{
			PinNumber:       "D4",
			NetName:         "EC_GPIO_D4",
			PinTypeOverride: "INPUT",
		},
		{
			PinNumber:       "D5",
			NetName:         "EC_GPIO_D5",
			PinTypeOverride: "INVALID_GPIO",
		},
	}

	var buf bytes.Buffer
	if err := GenerateDTSI(&buf, configs, chip); err != nil {
		t.Fatalf("GenerateDTSI failed: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "gpio_ec_gpio_d4: ec_gpio_d4") {
		t.Error("Expected D4 to be processed as GPIO because PinTypeOverride is INPUT")
	}
	if strings.Contains(out, "gpio_ec_gpio_d5") {
		t.Error("Expected D5 to NOT be processed as GPIO because PinTypeOverride is INVALID_GPIO")
	}
}

func TestGenerateDTSI_NonGPIOHardwarePinsAndUnusedPins(t *testing.T) {
	chip := &Chip{
		Name:   "ite-mock",
		Vendor: "ite",
		PinMap: map[string]string{
			"L1": "WRST#",
			"G6": "VCORE",
			"K6": "VCC1_RST#/GPO77",
			"M5": "PWM0/GPA0",
			"A6": "FMISO/GPG5",
			"M7": "PWM6/SSCK/GPA6",
		},
	}

	configs := []PinConfig{
		{PinNumber: "L1", NetName: "EC_ERST_L", Dir: "IN", PinTypeOverride: "INPUT_L"},
		{PinNumber: "G6", NetName: "VCORE_CAP", PinTypeOverride: "UNUSED_IN"},
		{PinNumber: "K6", NetName: "PLT_EC_RST_ODL", Dir: "IN", PinTypeOverride: "INPUT_L"},
		{PinNumber: "A6", NetName: "EN_PP5000_USBA_VBUS", Dir: "OUT", PinTypeOverride: "UNUSED_OUT", InitialCondition: "LOW", Pull: "DOWN"},
		{PinNumber: "M7", NetName: "NC", PinTypeOverride: "UNUSED_IN", Pull: "UP"},
		{PinNumber: "M5", NetName: "EC_VOLDN_BTN_ODL", PinTypeOverride: "UNUSED_IN"},
	}

	var buf bytes.Buffer
	if err := GenerateDTSI(&buf, configs, chip); err != nil {
		t.Fatalf("GenerateDTSI failed: %v", err)
	}

	out := buf.String()
	if strings.Contains(out, "ec_erst_l") || strings.Contains(out, "plt_ec_rst_odl") || strings.Contains(out, "en_pp5000_usba_vbus") {
		t.Errorf("Expected non-GPIO hardware pins, *EC_RST_ODL, and UNUSED_OUT to be excluded from named-gpios:\n%s", out)
	}

	expectedUnused := strings.Join([]string{
		"\tunused-pins {",
		"\t\tcompatible = \"unused-gpios\";",
		"\t\tunused-gpios =",
		"\t\t\t/* EC_VOLDN_BTN_ODL */",
		"\t\t\t<&gpioa 0 (GPIO_INPUT)>,",
		"\t\t\t/* NC */",
		"\t\t\t<&gpioa 6 (GPIO_INPUT | GPIO_PULL_UP)>,",
		"\t\t\t/* EN_PP5000_USBA_VBUS */",
		"\t\t\t<&gpiog 5 (GPIO_OUTPUT | GPIO_ACTIVE_HIGH | GPIO_PULL_DOWN | GPIO_OUTPUT_INIT_LOW)>;",
		"\t};",
	}, "\n")
	if !strings.Contains(out, expectedUnused) {
		t.Errorf("Expected unused-pins block:\n%s\nGot:\n%s", expectedUnused, out)
	}

	// Unknown pin not in chip.PinMap should still fail
	badConfigs := []PinConfig{{PinNumber: "Z99", NetName: "BAD_PIN", Dir: "IN"}}
	buf.Reset()
	if err := GenerateDTSI(&buf, badConfigs, chip); err == nil || !strings.Contains(err.Error(), "failed to resolve GPIO for pin Z99") {
		t.Errorf("Expected error for unknown pin Z99, got %v", err)
	}
}

func TestGenerateDTSI_PeripheralParity(t *testing.T) {
	t.Run("NPCX_KbdSleep_Tach_I2CCtrl", func(t *testing.T) {
		chip := &Chip{
			Name:   "npcx-mock",
			Vendor: "npcx",
			PinMap: map[string]string{
				"B2": "GPIOB4/I2C0_SDA0",
				"B3": "GPIOB5/I2C0_SCL0",
				"F5": "GPIOF5/TB1",
				"G1": "KSI0/GPIO31",
				"G2": "KSO01/GPIO21",
			},
		}
		configs := []PinConfig{
			// I2C_CLOCK without IO="OD" (e.g. Fatcat NPCX C12)
			{PinNumber: "B3", NetName: "EC_I2C_PD_SCL_R", Enum: "I2C_PORT_PD", PinTypeOverride: "I2C_CLOCK"},
			{PinNumber: "B2", NetName: "EC_I2C_PD_SDA_R", IO: "OD", PinTypeOverride: "I2C_DATA"},
			{PinNumber: "F5", NetName: "EC_FAN_TACH", PinTypeOverride: "TACH"},
			{PinNumber: "G1", NetName: "EC_KSI0", PinTypeOverride: "KEYBOARD"},
			{PinNumber: "G2", NetName: "EC_KSO01", PinTypeOverride: "KEYBOARD"},
		}

		var buf bytes.Buffer
		if err := GenerateDTSI(&buf, configs, chip); err != nil {
			t.Fatalf("GenerateDTSI failed: %v", err)
		}
		out := buf.String()

		expectedSnippets := []string{
			"&i2c_ctrl0 {\n\tstatus = \"okay\";\n};",
			"&i2c0_0 {",
			"port = <NPCX_TACH_PORT_B>;",
			"sample-clk = <NPCX_TACH_FREQ_LFCLK>;",
			"pulses-per-round = <2>;",
			"pinctrl-1 = <\n\t\t&ksi0_gpio_gp31\n\t\t&kso01_gpio_gp21\n\t>;",
			"pinctrl-names = \"default\", \"sleep\";",
		}
		for _, snip := range expectedSnippets {
			if !strings.Contains(out, snip) {
				t.Errorf("NPCX output missing expected snippet:\n%s\nGot:\n%s", snip, out)
			}
		}
	})

	t.Run("ITE_KbdSleep_Tach", func(t *testing.T) {
		chip := &Chip{
			Name:   "ite-mock",
			Vendor: "ite",
			PinMap: map[string]string{
				"D6": "TACH0A/GPD6",
				"G1": "KSI0/GPG1",
				"G2": "KSO0/GPG2",
			},
		}
		configs := []PinConfig{
			{PinNumber: "D6", NetName: "EC_FAN_TACH", PinTypeOverride: "TACH"},
			{PinNumber: "G1", NetName: "EC_KSI0", PinTypeOverride: "KEYBOARD"},
			{PinNumber: "G2", NetName: "EC_KSO0", PinTypeOverride: "KEYBOARD"},
		}

		var buf bytes.Buffer
		if err := GenerateDTSI(&buf, configs, chip); err != nil {
			t.Fatalf("GenerateDTSI failed: %v", err)
		}
		out := buf.String()

		expectedSnippets := []string{
			"channel = <IT8XXX2_TACH_CHANNEL_A>;",
			"pulses-per-round = <2>;",
			"pinctrl-1 = <\n\t\t&ksi0_sleep\n\t\t&kso0_sleep\n\t>;",
			"pinctrl-names = \"default\", \"sleep\";",
		}
		for _, snip := range expectedSnippets {
			if !strings.Contains(out, snip) {
				t.Errorf("ITE output missing expected snippet:\n%s\nGot:\n%s", snip, out)
			}
		}
	})
}

func TestGenerateDTSI_ITE_UART(t *testing.T) {
	chip := &Chip{
		Name:   "ite-mock",
		Vendor: "ite",
		PinMap: map[string]string{
			"A3": "SOUT0/GPB1",
			"A4": "SIN0/GPB0",
			"N3": "SOUT1/GPH2/ID2",
			"M3": "SIN1/GPH1/ID1",
		},
	}

	t.Run("NonDefaultUART2Only", func(t *testing.T) {
		configs := []PinConfig{
			{PinNumber: "N3", NetName: "UART_EC_TX_DBG_RX", PinTypeOverride: "UART_TX"},
			{PinNumber: "M3", NetName: "UART_DBG_TX_EC_RX", PinTypeOverride: "UART_RX"},
		}
		var buf bytes.Buffer
		if err := GenerateDTSI(&buf, configs, chip); err != nil {
			t.Fatalf("GenerateDTSI failed: %v", err)
		}
		out := buf.String()
		for _, snip := range []string{
			"&uart1 {\n\tstatus = \"disabled\";\n};",
			"&ite_uart1_wrapper {\n\tstatus = \"disabled\";\n};",
			"&uart2 {\n\tstatus = \"okay\";\n\tcurrent-speed = <115200>;\n};",
			"&ite_uart2_wrapper {\n\tstatus = \"okay\";\n\tpinctrl-0 = <\n\t\t&uart2_rx_gph1_default\n\t\t&uart2_tx_gph2_default\n\t>;\n\tpinctrl-names = \"default\";\n};",
		} {
			if !strings.Contains(out, snip) {
				t.Errorf("Expected non-default UART2 snippet:\n%s\nGot:\n%s", snip, out)
			}
		}
	})

	t.Run("DualUART_ConsoleOnUART1", func(t *testing.T) {
		configs := []PinConfig{
			{PinNumber: "A3", NetName: "UART_EC_TX_DBG_RX", PinTypeOverride: "UART_TX"},
			{PinNumber: "N3", NetName: "EC_BASE_TX", PinTypeOverride: "UART_TX"},
		}
		var buf bytes.Buffer
		if err := GenerateDTSI(&buf, configs, chip); err != nil {
			t.Fatalf("GenerateDTSI failed: %v", err)
		}
		out := buf.String()
		if strings.Contains(out, "&uart1") || strings.Contains(out, "&uart2") {
			t.Errorf("Expected no UART override when SOUT0 (uart1) is present, got:\n%s", out)
		}
	})
}
