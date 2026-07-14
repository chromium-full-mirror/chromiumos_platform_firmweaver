// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"encoding/csv"
	"fmt"
	"io"
	"strings"
)

type PinConfig struct {
	PinNumber        string
	PinName          string
	NetName          string
	Pull             string // UP, DOWN, N, ""
	Dir              string // IN, OUT, IO, ""
	IO               string // PWM, ADC, OD, TTL, UNUSED, ""
	Enum             string
	PinTypeOverride  string
	InitialCondition string // HIGH, LOW, ""
	NetNameOverride  string
	Voltage          string // 1.8V, 3.3V, ""
}

// Column aliases for CSV parsing. The first entry in each slice is the canonical column name,
// and the rest are alternative names that may appear in different CSV files.
// Add variants to the list as needed to support different CSV formats.
var columnAliases = map[string][]string{
	"PinNumber":        {"PinNumber", "Pin Number", "Pin", "Ball"},
	"PinName":          {"PinName", "Pin Name"},
	"NetName":          {"NetName", "Signal Name", "Net Name", "Net"},
	"Dir":              {"Dir", "DIR", "Direction"},
	"IO":               {"IO", "I/O Type", "IO Type", "I/O"},
	"Pull":             {"Pull", "Internal PU", "Pull Up", "Pull-Up"},
	"Voltage":          {"Voltage", "I/O Voltage", "Volts"},
	"Enum":             {"Enum", "Enum Name"},
	"PinTypeOverride":  {"PinTypeOverride", "Type", "SigType", "Pin Type Override"},
	"InitialCondition": {"InitialCondition", "Initial Condition", "Init State", "Initial State"},
	"NetNameOverride":  {"NetNameOverride", "Net Name Override"},
}

// ParseCSV parses EC pinout CSV data from an io.Reader.
func ParseCSV(reader io.Reader, pinCol string) ([]PinConfig, error) {
	r := csv.NewReader(reader)
	r.FieldsPerRecord = -1 // Allow variable number of fields per record

	// Read header
	header, err := r.Read()
	if err != nil {
		return nil, fmt.Errorf("failed to read header: %v", err)
	}

	findCol := func(candidates ...string) int {
		normalize := func(s string) string {
			return strings.Join(strings.Fields(s), " ")
		}

		// Pass 1: Exact match against header in original slice order
		for _, cand := range candidates {
			if cand == "" {
				continue
			}
			normCand := normalize(cand)
			for i, name := range header {
				if strings.EqualFold(normalize(name), normCand) {
					return i
				}
			}
		}
		// Pass 2: Substring match against header in original slice order
		for _, cand := range candidates {
			if cand == "" {
				continue
			}
			normCand := strings.ToLower(normalize(cand))
			for i, name := range header {
				if strings.Contains(strings.ToLower(normalize(name)), normCand) {
					return i
				}
			}
		}
		return -1
	}

	findColByAlias := func(key string, extraCandidates ...string) int {
		candidates := columnAliases[key]
		if len(extraCandidates) > 0 {
			candidates = append(extraCandidates, candidates...)
		}
		return findCol(candidates...)
	}

	primaryPinIdx := findColByAlias("PinNumber", pinCol)
	var fallbackPinIndices []int

	if primaryPinIdx >= 0 {
		primaryHeader := header[primaryPinIdx]
		parts := strings.Split(primaryHeader, "(")
		chipPrefix := strings.TrimSpace(parts[0])

		for i, name := range header {
			if i == primaryPinIdx {
				continue
			}
			nameLower := strings.ToLower(name)
			if (chipPrefix != "" && len(chipPrefix) > 2 && strings.Contains(nameLower, strings.ToLower(chipPrefix))) ||
				(pinCol != "" && strings.Contains(nameLower, strings.ToLower(pinCol))) {
				fallbackPinIndices = append(fallbackPinIndices, i)
			}
		}
	}

	netNameIdx := findColByAlias("NetName")
	dirIdx := findColByAlias("Dir")
	ioIdx := findColByAlias("IO")
	pullIdx := findColByAlias("Pull")
	voltageIdx := findColByAlias("Voltage")
	enumIdx := findColByAlias("Enum")
	pinTypeOverrideIdx := findColByAlias("PinTypeOverride")
	initialCondIdx := findColByAlias("InitialCondition")
	netNameOverrideIdx := findColByAlias("NetNameOverride")
	pinNameIdx := findColByAlias("PinName")

	if primaryPinIdx == -1 {
		return nil, fmt.Errorf("missing required column for Pin Number (checked --pin-col=%q, 'PinNumber', 'Pin')", pinCol)
	}
	if netNameIdx == -1 {
		return nil, fmt.Errorf("missing required column for Net Name (checked 'NetName', 'Signal Name')")
	}

	var configs []PinConfig
	lineNum := 1

	for {
		row, err := r.Read()
		lineNum++
		if err == io.EOF {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("failed to read row at line %d: %v", lineNum, err)
		}

		getValue := func(idx int) string {
			if idx >= 0 && idx < len(row) {
				return strings.TrimSpace(row[idx])
			}
			return ""
		}

		netName := getValue(netNameIdx)
		netUpper := strings.ToUpper(netName)
		if netName == "" || netUpper == "NC" || netUpper == "NC (TP)" || netUpper == "NC(TP)" ||
			netUpper == "NC_TP" || strings.HasPrefix(netUpper, "NC_") || strings.HasPrefix(netUpper, "NC ") ||
			netUpper == "OPEN" || netUpper == "(OPEN)" || netUpper == "FLOATING" || netUpper == "(OPEN, FLOATING)" ||
			netUpper == "N/A" || netUpper == "NA" {
			continue
		}

		rawDir := getValue(dirIdx)
		rawIO := getValue(ioIdx)
		rawPull := getValue(pullIdx)
		rawVoltage := getValue(voltageIdx)
		enumVal := getValue(enumIdx)
		sigType := getValue(pinTypeOverrideIdx)
		initialCond := getValue(initialCondIdx)
		netNameOverride := getValue(netNameOverrideIdx)
		pinName := getValue(pinNameIdx)

		pinNum := getValue(primaryPinIdx)
		if pinNum == "" || pinNum == "N/A" || pinNum == "#N/A" || pinNum == "--" {
			// Fall back to sibling variant columns for required peripheral or explicit enum signals
			isPeripheralOrEnum := enumVal != "" || rawIO == "ADC" || rawIO == "I2C" || rawIO == "PWM" || rawIO == "TACH" ||
				sigType == "ADC" || sigType == "I2C_CLOCK" || sigType == "I2C_DATA" || sigType == "PWM"
			if isPeripheralOrEnum {
				for _, fbIdx := range fallbackPinIndices {
					val := getValue(fbIdx)
					if val != "" && val != "N/A" && val != "#N/A" && val != "--" {
						pinNum = val
						break
					}
				}
			}
		}

		if pinNum == "" || pinNum == "N/A" || pinNum == "#N/A" || pinNum == "--" {
			continue
		}

		// Normalize Direction
		direction := ""
		if rawDir == "OUT" {
			direction = "OUT"
		} else if rawDir == "IN" {
			direction = "IN"
		} else if rawDir == "I/O" || rawDir == "IO" {
			if strings.HasPrefix(sigType, "OUTPUT") || strings.HasPrefix(sigType, "ODR") {
				direction = "OUT"
			} else {
				direction = "IN"
			}
		}

		// Normalize Pull
		pull := rawPull
		if rawPull == "Y" {
			pull = "UP"
		} else if rawPull == "N" {
			pull = "N"
		}

		// Normalize Voltage
		voltage := rawVoltage
		if strings.Contains(rawVoltage, "1.8") {
			voltage = "1.8V"
		} else if strings.Contains(rawVoltage, "3.3") {
			voltage = "3.3V"
		}

		// Normalize Pin Type & Initial Condition
		pinType := sigType
		if pinType == "INPUT_PU" {
			pinType = "INPUT"
			pull = "UP"
			if strings.HasSuffix(netName, "_L") || strings.HasSuffix(netName, "_ODL") || strings.HasSuffix(netName, "_LOW") {
				pinType = "INPUT_L"
			}
		} else if pinType == "INPUT" && direction == "IN" {
			if strings.HasSuffix(netName, "_L") || strings.HasSuffix(netName, "_ODL") || strings.HasSuffix(netName, "_LOW") {
				pinType = "INPUT_L"
			}
		} else if pinType == "OUTPUT_L" && initialCond == "" {
			initialCond = "LOW"
		} else if pinType == "OUTPUT_H" && initialCond == "" {
			initialCond = "HIGH"
		} else if pinType == "OUTPUT_ODL" && initialCond == "" {
			initialCond = "LOW"
		} else if pinType == "OUTPUT_ODR" && initialCond == "" {
			initialCond = "HIGH"
		}

		// Fix known signal defaults where ODR outputs are active high init in Zephyr
		if netName == "EC_PCH_WAKE_ODL" && initialCond == "" {
			initialCond = "HIGH"
		}

		// Clean double quotes in enum
		enumVal = strings.ReplaceAll(enumVal, `""`, `"`)
		enumVal = strings.Trim(enumVal, `"`)

		config := PinConfig{
			PinNumber:        pinNum,
			PinName:          pinName,
			NetName:          netName,
			Pull:             pull,
			Dir:              direction,
			IO:               rawIO,
			Enum:             enumVal,
			PinTypeOverride:  pinType,
			InitialCondition: initialCond,
			NetNameOverride:  netNameOverride,
			Voltage:          voltage,
		}

		configs = append(configs, config)
	}

	return configs, nil
}
