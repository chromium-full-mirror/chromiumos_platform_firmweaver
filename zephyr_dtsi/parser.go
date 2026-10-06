// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zephyr_dtsi

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

// Column aliases for CSV/sheet parsing.
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

func normalize(s string) string {
	return strings.Join(strings.Fields(s), " ")
}

// findCol searches header for candidates using exact match then substring match (for length >= 4).
func findCol(header []string, candidates ...string) int {
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
	for _, cand := range candidates {
		if len(cand) < 4 {
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

func findPinCol(header []string, pinCol string) int {
	if pinCol != "" {
		return findCol(header, pinCol)
	}
	for _, cand := range columnAliases["PinNumber"] {
		normCand := normalize(cand)
		for i, name := range header {
			if strings.EqualFold(normalize(name), normCand) {
				return i
			}
		}
	}
	for i, name := range header {
		lower := strings.ToLower(name)
		if strings.Contains(lower, "name") || strings.Contains(lower, "type") || strings.Contains(lower, "desc") {
			continue
		}
		if strings.Contains(lower, "pin") || strings.Contains(lower, "bga") || strings.Contains(lower, "ball") {
			return i
		}
	}
	return -1
}

// findHeaderRow scans the first 10 rows for a row containing both Pin and Net columns.
func findHeaderRow(records [][]string, pinCol string) ([]string, [][]string, error) {
	if len(records) == 0 {
		return nil, nil, fmt.Errorf("failed to read header: empty records")
	}

	limit := 10
	if len(records) < limit {
		limit = len(records)
	}

	for i := 0; i < limit; i++ {
		row := records[i]
		if len(row) < 2 {
			continue
		}

		pinIdx := findPinCol(row, pinCol)
		netIdx := findCol(row, columnAliases["NetName"]...)

		if pinIdx >= 0 && netIdx >= 0 {
			return row, records[i+1:], nil
		}
	}

	// If a row clearly represents a header by having NetName or PinNumber, provide a precise diagnostic error.
	for i := 0; i < limit; i++ {
		row := records[i]
		netIdx := findCol(row, columnAliases["NetName"]...)
		pinIdx := findPinCol(row, pinCol)
		if netIdx >= 0 && pinIdx == -1 {
			if pinCol != "" {
				return nil, nil, fmt.Errorf("missing required column for Pin Number (checked --pin-col=%q)", pinCol)
			}
			return nil, nil, fmt.Errorf("missing required column for Pin Number (checked 'PinNumber', 'Pin', 'Ball', package headers)")
		}
		if pinIdx >= 0 && netIdx == -1 {
			return nil, nil, fmt.Errorf("missing required column for Net Name (checked 'NetName', 'Signal Name')")
		}
	}

	var inspected []string
	for i := 0; i < limit; i++ {
		preview := strings.Join(records[i], ", ")
		runes := []rune(preview)
		if len(runes) > 100 {
			preview = string(runes[:100]) + "..."
		}
		inspected = append(inspected, fmt.Sprintf("Row %d: %q", i+1, preview))
	}

	return nil, nil, fmt.Errorf(
		"unable to locate pinout table header in first %d rows.\n"+
			"Expected columns: Pin Number (e.g. 'PinNumber', 'Pin', or --pin-col) AND Net Name (e.g. 'NetName', 'Signal Name').\n"+
			"Inspected rows:\n  %s",
		limit, strings.Join(inspected, "\n  "),
	)
}

// isIgnoredSignal returns true for empty, formula error, NC, or open/floating signals.
func isIgnoredSignal(netName string) bool {
	if netName == "" || strings.HasPrefix(netName, "#") {
		return true
	}
	upper := strings.ToUpper(netName)
	switch upper {
	case "NC", "NC (TP)", "NC(TP)", "NC_TP", "OPEN", "(OPEN)", "FLOATING", "(OPEN, FLOATING)", "N/A", "NA":
		return true
	}
	return strings.HasPrefix(upper, "NC_") || strings.HasPrefix(upper, "NC ")
}

// isValidPin returns true if pin is non-empty, not a formula error, and not a placeholder.
func isValidPin(pin string) bool {
	if pin == "" || strings.HasPrefix(pin, "#") {
		return false
	}
	switch pin {
	case "N/A", "--", "-":
		return false
	default:
		return true
	}
}

// isSpecialPeripheral returns true if the pin has special hardware function requirements.
func isSpecialPeripheral(enumVal, rawIO, sigType string) bool {
	if enumVal != "" {
		return true
	}
	switch rawIO {
	case "ADC", "I2C", "PWM", "TACH":
		return true
	}
	switch sigType {
	case "ADC", "I2C_CLOCK", "I2C_DATA", "PWM":
		return true
	}
	return false
}

// normalizeDirection maps direction strings to canonical "IN", "OUT", or "".
func normalizeDirection(rawDir, sigType string) string {
	upper := strings.ToUpper(strings.TrimSpace(rawDir))
	switch upper {
	case "OUT":
		return "OUT"
	case "IN":
		return "IN"
	case "I/O", "IO", "BIDIR":
		if strings.HasPrefix(sigType, "OUTPUT") || strings.HasPrefix(sigType, "ODR") {
			return "OUT"
		}
		return "IN"
	}
	return ""
}

// normalizePinType resolves active-low signal types, pull defaults, and initial states.
func normalizePinType(sigType, dir, netName, initCond, rawPull string) (string, string, string) {
	pull := strings.ToUpper(strings.TrimSpace(rawPull))
	if pull == "Y" {
		pull = "UP"
	} else if pull == "N" {
		pull = "N"
	}

	pinType := sigType
	isLow := strings.HasSuffix(netName, "_L") || strings.HasSuffix(netName, "_ODL") || strings.HasSuffix(netName, "_LOW")

	if pinType == "INPUT_PU" {
		pinType = "INPUT"
		pull = "UP"
		if isLow {
			pinType = "INPUT_L"
		}
	} else if pinType == "INPUT" && dir == "IN" && isLow {
		pinType = "INPUT_L"
	} else if initCond == "" {
		switch pinType {
		case "OUTPUT_L", "OUTPUT_ODL":
			initCond = "LOW"
		case "OUTPUT_H", "OUTPUT_ODR":
			initCond = "HIGH"
		}
	}

	if netName == "EC_PCH_WAKE_ODL" && initCond == "" {
		initCond = "HIGH"
	}

	return pinType, pull, initCond
}

func normalizeVoltage(raw string) string {
	if strings.Contains(raw, "1.8") {
		return "1.8V"
	}
	if strings.Contains(raw, "3.3") {
		return "3.3V"
	}
	return raw
}

// ParseCSV parses EC pinout CSV data from an io.Reader.
func ParseCSV(reader io.Reader, pinCol string) ([]PinConfig, error) {
	r := csv.NewReader(reader)
	r.FieldsPerRecord = -1

	var records [][]string
	lineNum := 0
	for {
		lineNum++
		row, err := r.Read()
		if err == io.EOF {
			break
		}
		if err != nil {
			if lineNum == 1 {
				return nil, fmt.Errorf("failed to read header: %w", err)
			}
			return nil, fmt.Errorf("failed to read row at line %d: %w", lineNum, err)
		}
		records = append(records, row)
	}

	if len(records) == 0 {
		return nil, fmt.Errorf("failed to read header: empty CSV input")
	}

	return ParseRecords(records, pinCol)
}

// ParseRecords parses 2D spreadsheet records into PinConfig structures.
func ParseRecords(records [][]string, pinCol string) ([]PinConfig, error) {
	if len(records) == 0 {
		return nil, fmt.Errorf("failed to read header: empty records")
	}

	header, dataRows, err := findHeaderRow(records, pinCol)
	if err != nil {
		return nil, err
	}

	return parseRows(header, dataRows, pinCol)
}

func parseRows(header []string, rows [][]string, pinCol string) ([]PinConfig, error) {
	col := func(key string) int {
		return findCol(header, columnAliases[key]...)
	}

	pinIdx := findPinCol(header, pinCol)
	netIdx := col("NetName")

	if pinIdx < 0 || pinIdx >= len(header) {
		return nil, fmt.Errorf("missing required Pin column in header")
	}
	if netIdx < 0 || netIdx >= len(header) {
		return nil, fmt.Errorf("missing required Net column in header")
	}

	dirIdx := col("Dir")
	ioIdx := col("IO")
	pullIdx := col("Pull")
	voltIdx := col("Voltage")
	enumIdx := col("Enum")
	typeIdx := col("PinTypeOverride")
	initIdx := col("InitialCondition")
	netOverrideIdx := col("NetNameOverride")
	pinNameIdx := col("PinName")

	var fallbackPinIndices []int
	primaryHeader := header[pinIdx]
	var chipPrefix string
	if strings.Contains(primaryHeader, "(") {
		chipPrefix = strings.TrimSpace(strings.Split(primaryHeader, "(")[0])
	}
	for i, name := range header {
		if i == pinIdx {
			continue
		}
		lower := strings.ToLower(name)
		if strings.Contains(lower, "name") || strings.Contains(lower, "type") || strings.Contains(lower, "desc") {
			continue
		}
		if (chipPrefix != "" && len(chipPrefix) > 2 && strings.Contains(lower, strings.ToLower(chipPrefix))) ||
			(pinCol != "" && strings.Contains(lower, strings.ToLower(pinCol))) {
			fallbackPinIndices = append(fallbackPinIndices, i)
		}
	}

	cell := func(row []string, idx int) string {
		if idx >= 0 && idx < len(row) {
			return strings.TrimSpace(row[idx])
		}
		return ""
	}

	var configs []PinConfig
	for _, row := range rows {
		netName := cell(row, netIdx)
		rawIO := strings.ToUpper(cell(row, ioIdx))
		sigType := strings.ToUpper(cell(row, typeIdx))
		if sigType != "UNUSED_IN" && sigType != "UNUSED_OUT" && isIgnoredSignal(netName) {
			continue
		}

		rawDir := cell(row, dirIdx)
		enumVal := strings.Trim(strings.ReplaceAll(cell(row, enumIdx), `""`, `"`), `"`)

		pinNum := cell(row, pinIdx)
		if !isValidPin(pinNum) && isSpecialPeripheral(enumVal, rawIO, sigType) {
			for _, fb := range fallbackPinIndices {
				if val := cell(row, fb); isValidPin(val) {
					pinNum = val
					break
				}
			}
		}
		if !isValidPin(pinNum) {
			continue
		}

		dir := normalizeDirection(rawDir, sigType)
		pinType, pull, initCond := normalizePinType(sigType, dir, netName, cell(row, initIdx), cell(row, pullIdx))

		configs = append(configs, PinConfig{
			PinNumber:        pinNum,
			PinName:          cell(row, pinNameIdx),
			NetName:          netName,
			Pull:             pull,
			Dir:              dir,
			IO:               rawIO,
			Enum:             enumVal,
			PinTypeOverride:  pinType,
			InitialCondition: initCond,
			NetNameOverride:  cell(row, netOverrideIdx),
			Voltage:          normalizeVoltage(cell(row, voltIdx)),
		})
	}

	if len(configs) == 0 {
		return nil, fmt.Errorf("parsed 0 valid pin configurations from sheet/CSV. Check that rows contain valid non-empty pin numbers and signal names")
	}

	return configs, nil
}
