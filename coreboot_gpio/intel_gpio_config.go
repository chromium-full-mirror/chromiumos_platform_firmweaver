// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coreboot_gpio

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"strings"
)

// IntelGPIOPin represents a single Intel-specific GPIO pin configuration.
type IntelGPIOPin struct {
	GPIO        string
	SignalName  string
	NetName     string
	PadRstCfg   string
	Pmode       string
	GpioDir     string
	GPIOTxState string // Added GPIOTxState
}

var pmodeNativeFuncRegex = regexp.MustCompile(`Native Function (\d+)`) // Regex to extract number from Pmode

func parseIntelCSV(records [][]string) ([]IntelGPIOPin, error) {
	const headerSearchLimit = 3 // Search the first 3 rows for header info

	if len(records) < headerSearchLimit {
		return nil, fmt.Errorf("file has fewer than %d rows, cannot parse headers", headerSearchLimit)
	}

	// Determine the width of the header by finding the longest of the first few rows
	headerWidth := 0
	for i := 0; i < headerSearchLimit; i++ {
		if len(records[i]) > headerWidth {
			headerWidth = len(records[i])
		}
	}

	// Unify the first N rows into a single logical header, combining text from the same column
	unifiedHeader := make([]string, headerWidth)
	for col := 0; col < headerWidth; col++ {
		var combinedHeaderCell strings.Builder
		for row := 0; row < headerSearchLimit; row++ {
			if col < len(records[row]) && strings.TrimSpace(records[row][col]) != "" {
				if combinedHeaderCell.Len() > 0 {
					combinedHeaderCell.WriteString(" ") // Add a space between parts from different rows
				}
				combinedHeaderCell.WriteString(strings.TrimSpace(records[row][col]))
			}
		}
		unifiedHeader[col] = combinedHeaderCell.String()
	}

	// The actual data starts after the header rows we've processed
	dataRows := records[headerSearchLimit:]

	// Map expected header names to their actual column index using the unified header
	colIndexMap := make(map[string]int)
	expectedHeaders := []string{"GPIO", "Signal Name", "Net Name", "PadRstCfg", "Pmode", "Gpio Pin Direction", "Gpio Output State"}

	for _, expected := range expectedHeaders {
		found := false
		for i, actual := range unifiedHeader {
			if strings.Contains(actual, expected) {
				colIndexMap[expected] = i
				found = true
				break
			}
		}
		if !found {
			return nil, fmt.Errorf("expected header containing '%s' not found in the first %d rows", expected, headerSearchLimit)
		}
	}

	var gpioPins []IntelGPIOPin
	for i, row := range dataRows {
		// Check that the row is not empty or just commas
		isRowEmpty := true
		for _, cell := range row {
			if strings.TrimSpace(cell) != "" {
				isRowEmpty = false
				break
			}
		}
		if isRowEmpty {
			continue // Skip empty or effectively empty rows
		}

		// Ensure the row has enough columns to prevent a panic
		maxIndex := 0
		for _, index := range colIndexMap {
			if index > maxIndex {
				maxIndex = index
			}
		}
		if len(row) <= maxIndex {
			fmt.Fprintf(os.Stderr, "Warning: skipping malformed row %d: not enough columns\n", headerSearchLimit+i+1)
			continue
		}

		gpioPins = append(gpioPins, IntelGPIOPin{
			GPIO:        row[colIndexMap["GPIO"]],
			SignalName:  row[colIndexMap["Signal Name"]],
			NetName:     row[colIndexMap["Net Name"]],
			PadRstCfg:   row[colIndexMap["PadRstCfg"]],
			Pmode:       row[colIndexMap["Pmode"]],
			GpioDir:     row[colIndexMap["Gpio Pin Direction"]],
			GPIOTxState: row[colIndexMap["Gpio Output State"]],
		})
	}
	return gpioPins, nil
}

// mapPadRstCfgToCIdentifier maps PadRstCfg string values to C identifiers.
func mapPadRstCfgToCIdentifier(padRstCfg string) string {
	switch strings.TrimSpace(padRstCfg) {
	case "GPIO reset", "ResetResume": // Added "ResetResume"
		return "PLTRST"
	case "Deep GPIO reset", "Default":
		return "DEEP"
	default:
		// Fallback for unexpected values, or if the field is empty
		if strings.TrimSpace(padRstCfg) == "" {
			return "NONE" // Assuming NONE for empty PadRstCfg
		}
		fmt.Fprintf(os.Stderr, "Warning: Unknown PadRstCfg value '%s', using original.\n", padRstCfg)
		return padRstCfg // Fallback to original if not mapped
	}
}

// mapGPIOTxStateToCIdentifier maps GPIOTxState string values to C identifiers.
func mapGPIOTxStateToCIdentifier(gpioTxState string) string {
	switch strings.TrimSpace(gpioTxState) {
	case "High", "HIGH":
		return "1"
	case "Low", "LOW":
		return "0"
	default:
		// Fallback for unexpected values, or if the field is empty
		if strings.TrimSpace(gpioTxState) == "" {
			return "NONE" // Assuming NONE for empty GPIOTxState
		}
		fmt.Fprintf(os.Stderr, "Warning: Unknown GPIOTxState value '%s', using original.\n", gpioTxState)
		return gpioTxState // Fallback to original if not mapped
	}
}

// processIntelGPIOs processes the Intel GPIO records and writes the C config to outputWriter.
func processIntelGPIOs(records [][]string, outputWriter io.Writer) error {
	intelPins, err := parseIntelCSV(records)
	if err != nil {
		return fmt.Errorf("error parsing Intel CSV: %w", err)
	}

	filteredPins := filterIntelPins(intelPins)

	fmt.Fprintln(os.Stderr, "Successfully parsed Intel GPIO pin data. Generating C source...")

	writeCBoilerplate(outputWriter)

	for _, pin := range filteredPins {
		writePinConfig(outputWriter, pin)
	}

	fmt.Fprintln(outputWriter, "};")
	return nil
}

// filterIntelPins filters out pins that don't need configuration.
func filterIntelPins(pins []IntelGPIOPin) []IntelGPIOPin {
	var filteredPins []IntelGPIOPin
	for _, pin := range pins {
		// Condition for "no configuration":
		// NetName contains "GPP_DOES_NOT EXIST" AND Pmode is empty AND PadRstCfg is empty
		if strings.Contains(pin.NetName, "GPP_DOES_NOT EXIST") &&
			strings.TrimSpace(pin.Pmode) == "" &&
			strings.TrimSpace(pin.PadRstCfg) == "" {
			// Skip this pin
			fmt.Fprintf(os.Stderr, "Skipping pin %s (no configuration needed).\n", pin.GPIO)
			continue
		}
		filteredPins = append(filteredPins, pin)
	}
	return filteredPins
}

// writeCBoilerplate writes the header of the generated C file.
func writeCBoilerplate(w io.Writer) {
	fmt.Fprintln(w, "// Generated by coreboot_gpio")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static const struct pad_config gpio_table[] = {")
}

// writePinConfig writes the configuration for a single pin.
func writePinConfig(w io.Writer, pin IntelGPIOPin) {
	fmt.Fprintf(w, "\t/* %s:     %s */\n", pin.GPIO, pin.NetName)
	if strings.Contains(pin.SignalName, "ESPI_") {
		fmt.Fprintf(w, "\t/* %s => %s configured on reset, do not touch */\n\n", pin.GPIO, pin.NetName)
		return
	}

	if strings.Contains(pin.Pmode, "NC") {
		fmt.Fprintf(w, "\tPAD_NC(%s, NONE),\n", pin.GPIO)
		return
	}

	if strings.Contains(pin.Pmode, "Native Function") {
		writeNativeFuncConfig(w, pin)
		return
	}

	if strings.TrimSpace(pin.Pmode) == "GPIO" {
		writeGPIOConfig(w, pin)
		return
	}

	// Placeholder for other pins
	fmt.Fprintf(w, "\t// TODO: Process pin %s\n", pin.GPIO)
}

func writeNativeFuncConfig(w io.Writer, pin IntelGPIOPin) {
	matches := pmodeNativeFuncRegex.FindStringSubmatch(pin.Pmode)
	if len(matches) < 2 {
		fmt.Fprintf(w, "\t// TODO: Could not extract number from Native Function Pmode for pin %s: %s\n", pin.GPIO, pin.Pmode)
		return
	}

	pmodeNum := matches[1]
	if strings.Contains(pin.NetName, "CNV") {
		fmt.Fprintf(w, "\tPAD_CFG_NF_IOSTANDBY_IGNORE(%s, NONE, DEEP, NF%s),\n", pin.GPIO, pmodeNum)
	} else {
		fmt.Fprintf(w, "\tPAD_CFG_NF(%s, NONE, DEEP, NF%s),\n", pin.GPIO, pmodeNum)
	}
}

func writeGPIOConfig(w io.Writer, pin IntelGPIOPin) {
	if strings.TrimSpace(pin.GpioDir) == "OUT" {
		mappedPadRst := mapPadRstCfgToCIdentifier(pin.PadRstCfg)
		mappedGPIOTxState := mapGPIOTxStateToCIdentifier(pin.GPIOTxState)
		fmt.Fprintf(w, "\tPAD_CFG_GPO(%s, %s, %s),\n", pin.GPIO, mappedGPIOTxState, mappedPadRst)
		return
	}

	if strings.TrimSpace(pin.GpioDir) == "IN" {
		writeGPIInConfig(w, pin)
		return
	}
}

func writeGPIInConfig(w io.Writer, pin IntelGPIOPin) {
	if strings.Contains(pin.NetName, "PCIE_WAKE_ODL") {
		fmt.Fprintf(w, "\tPAD_CFG_GPI_SCI_LOW(%s, NONE, DEEP, LEVEL),\n", pin.GPIO)
	} else if strings.Contains(pin.NetName, "EC_SOC_INT_ODL") ||
		strings.Contains(pin.NetName, "GSC_SOC_INT_ODL") {
		fmt.Fprintf(w, "\tPAD_CFG_GPI_APIC_LOCK(%s, NONE, LEVEL, INVERT, LOCK_CONFIG),\n", pin.GPIO)
	} else if strings.Contains(pin.NetName, "PEN_DETECT") {
		fmt.Fprintf(w, "\tPAD_CFG_GPI_TRIG_OWN(%s, NONE, DEEP, LEVEL, ACPI),\n", pin.GPIO)
	} else if strings.Contains(pin.NetName, "FP") && strings.Contains(pin.NetName, "INT") {
		fmt.Fprintf(w, "\tPAD_CFG_GPI_IRQ_WAKE(%s, NONE, PWROK, LEVEL, INVERT),\n", pin.GPIO)
	} else if (strings.Contains(pin.NetName, "TP") || strings.Contains(pin.NetName, "TCHPAD") || strings.Contains(pin.NetName, "TCH_PAD")) && strings.Contains(pin.NetName, "INT") {
		fmt.Fprintf(w, "\tPAD_CFG_GPI_APIC_DRIVER(%s, NONE, PLTRST, EDGE_SINGLE, INVERT),\n", pin.GPIO)
	} else if strings.Contains(pin.NetName, "EC_SOC_WAKE_ODL") {
		fmt.Fprintf(w, "\tPAD_CFG_NF(%s, NONE, DEEP, NF1),\n", pin.GPIO)
	} else if strings.Contains(pin.NetName, "INT") {
		// Placeholder for Pmode=GPIO, GpioDir=IN and INT in NetName
		fmt.Fprintf(w, "\t// TODO: Check pin %s interrupt route, polarity, trigger\n", pin.GPIO)
		fmt.Fprintf(w, "\tPAD_CFG_GPI_INT(%s, NONE, PLTRST, EDGE_BOTH),\n", pin.GPIO)
	} else {
		fmt.Fprintf(w, "\tPAD_CFG_GPI(%s, NONE, DEEP),\n", pin.GPIO)
	}
}
