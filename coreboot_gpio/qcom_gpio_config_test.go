// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coreboot_gpio

import (
	"bytes"
	"encoding/csv"
	"os"
	"strings"
	"testing"
)

func TestProcessQcomGPIOs_ExampleCSV(t *testing.T) {
	f, err := os.Open("test/ExampleQcomGpio.csv")
	if err != nil {
		t.Fatalf("failed to open ExampleQcomGpio.csv: %v", err)
	}
	defer f.Close()

	reader := csv.NewReader(f)
	reader.FieldsPerRecord = -1
	records, err := reader.ReadAll()
	if err != nil {
		t.Fatalf("failed to read CSV records: %v", err)
	}

	var buf bytes.Buffer
	if err := processQcomGPIOs(records, &buf); err != nil {
		t.Fatalf("processQcomGPIOs() failed: %v", err)
	}

	out := buf.String()

	expectedSnippets := []string{
		// Section headers
		"Section 1: board.h - Symbolic GPIO Definitions",
		"Section 2: early_gpio.c - Early Romstage Default GPIO Configuration Table",
		"Section 3: chromeos.c - ChromeOS GPIO Setup & Coreboot Table Handoff",
		"Section 4: bootblock.c - Bootblock GPIO & Early QUP Bus Initialization",
		"Section 5: romstage.c - Romstage Peripheral Power & Reset Sequencing",
		"Section 6: mainboard.c - Ramstage Peripherals, Audio EGPIO & QUP SE FW",

		// Section 1: board.h macros
		"#define GPIO_AP_EC_INT                   GPIO(67)",
		"#define GPIO_GSC_AP_INT                  GPIO(91)",
		"#define GPIO_EN_FP_RAILS                 GPIO(22)",
		"#define GPIO_FP_RST_L                    GPIO(25)",
		"#define GPIO_FPMCU_BOOT0                 GPIO(24)",
		"#define GPIO_FPMCU_INT                   GPIO(23)",
		"#define GPIO_PANEL_POWER_ON              GPIO(70)",
		"#define GPIO_PANEL_HPD                   GPIO(119)",
		"#define GPIO_LID_OPEN_S3                 GPIO(92)",
		"#define GPIO_SD_CD_L                     GPIO(71)",
		"#define GPIO_USB_C1_EN_PP3300            GPIO(186)",
		"#define GPIO_USB_C1_EN_PP1800            GPIO(175)",
		"#define GPIO_USB_C1_EN_PP0900            GPIO(188)",
		"#define GPIO_USB_C1_RETIMER_RESET_L      GPIO(176)",
		"#define GPIO_USB_CAM_RESET_L             GPIO(10)",
		"#define GPIO_USB_CAM_ENABLE              GPIO(206)",
		"#define GPIO_SNDW_0_SCL                  GPIO(202)",
		"#define GPIO_FUN_SNDW_0_SCL              4",
		"#define GPIO_SNDW_0_SDA                  GPIO(203)",
		"#define GPIO_FUN_SNDW_0_SDA              3",
		"#define GPIO_SNDW_AMP_0_ENABLE           GPIO(204)",
		"#define GPIO_SNDW_AMP_1_ENABLE           GPIO(205)",

		// Section 2: early_gpio.c
		"GPIO_CFG_IN(GPIO_TPAD_INT_L)",
		"GPIO_CFG_IN(GPIO_AP_WP_ODL)",
		"GPIO_CFG_IN(GPIO_SD_CD_L)",
		"GPIO_CFG_IN(GPIO_LID_OPEN_S3)",
		"GPIO_CFG_OUT_LOW(GPIO_TS0_RESET_L)",
		"void configure_gpios_defaults(void)",

		// Section 3: chromeos.c
		"gpio_input_pullup(GPIO_AP_EC_INT);",
		"gpio_input_irq(GPIO_GSC_AP_INT, IRQ_TYPE_RISING_EDGE, GPIO_PULL_UP);",
		"gpio_input_irq(GPIO_FPMCU_INT, IRQ_TYPE_LEVEL, GPIO_PULL_UP);",
		"void fill_lb_gpios(struct lb_gpios *gpios)",

		// Section 4: bootblock.c
		"i2c_init(QUPV3_1_SE2, I2C_SPEED_FAST); /* H1/TPM I2C */",
		"qup_spi_init(QUPV3_1_SE5, 1010 * KHz); /* EC SPI */",

		// Section 5: romstage.c
		"configure_gpios_defaults();",
		"edp_configure_gpios();",
		"early_setup_usb_typec();",
		"void platform_romstage_postram(void)",

		// Section 6: mainboard.c
		"gpio_configure_no_egpio(GPIO_SNDW_0_SCL, GPIO_FUN_SNDW_0_SCL,",
		"gpio_configure_no_egpio(GPIO_SNDW_0_SDA, GPIO_FUN_SNDW_0_SDA,",
		"qupv3_se_fw_load_and_init(QUPV3_2_SE4, SE_PROTOCOL_I2C, MIXED); /* ADSP I2C (Charger/Fuel gauge) */",
		"qupv3_se_fw_load_and_init(QUPV3_0_SE0, SE_PROTOCOL_I2C, MIXED); /* Trackpad I2C */",
		"i2c_init(QUPV3_0_SE3, I2C_SPEED_FAST); /* USB-C0 Re-Timer I2C */",
		"i2c_init(QUPV3_0_SE7, I2C_SPEED_FAST); /* USB-C1 Re-Timer I2C */",
	}

	for _, snippet := range expectedSnippets {
		if !strings.Contains(out, snippet) {
			t.Errorf("expected generated output to contain %q, but it was missing.\nFull output:\n%s", snippet, out)
		}
	}
}

func TestParseQcomCSV_Errors(t *testing.T) {
	if _, err := parseQcomCSV([][]string{{"only_one_row"}}); err == nil {
		t.Errorf("expected error for fewer than 2 rows")
	}
	if _, err := parseQcomCSV([][]string{{"ColA", "ColB"}, {"1", "2"}}); err == nil {
		t.Errorf("expected error when 'Pad name' and 'Pull' columns are missing")
	}
	if _, err := parseQcomCSV([][]string{{"Pad name", "Pull", "CRD Net Name"}, {"GPIO_0", "PD", "NET0"}}); err == nil {
		t.Errorf("expected error when only reference CRD net name column is present")
	}
}
