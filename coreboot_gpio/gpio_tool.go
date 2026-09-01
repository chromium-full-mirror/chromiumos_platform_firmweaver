// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"io"
	"net/url"
	"os"
	"strconv"
	"strings"
)

// GPIOPin represents a single generic GPIO pin configuration.
type GPIOPin struct {
	Name     string
	Pin      int
	Function string
}

// extractSheetInfo extracts the spreadsheet ID from a Google Sheet URL.
func extractSheetInfo(sheetURL string) (string, error) {
	u, err := url.Parse(sheetURL)
	if err != nil {
		return "", fmt.Errorf("invalid URL: %w", err)
	}

	if u.Host != "docs.google.com" {
		return "", fmt.Errorf("not a Google Sheet URL")
	}

	parts := strings.Split(u.Path, "/")
	if len(parts) < 4 || parts[1] != "spreadsheets" || parts[2] != "d" {
		return "", fmt.Errorf("URL does not match expected Google Sheet format")
	}

	spreadsheetID := parts[3]
	return spreadsheetID, nil
}

func parseGenericCSV(records [][]string) ([]GPIOPin, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV data must have a header row and at least one data row")
	}

	header := records[0]
	expectedHeaders := []string{"GPIO_NAME", "PIN_NUMBER", "FUNCTION"}
	if len(header) < len(expectedHeaders) || header[0] != expectedHeaders[0] || header[1] != expectedHeaders[1] || header[2] != expectedHeaders[2] {
		return nil, fmt.Errorf("unexpected CSV header format. Expected '%s', but got '%s'", strings.Join(expectedHeaders, ","), strings.Join(header, ","))
	}

	var gpioPins []GPIOPin
	for i, row := range records[1:] {
		if len(row) < len(expectedHeaders) {
			fmt.Fprintf(os.Stderr, "Warning: skipping malformed row %d: not enough columns\n", i+2)
			continue
		}

		pin, err := strconv.Atoi(row[1])
		if err != nil {
			fmt.Fprintf(os.Stderr, "Warning: skipping row %d: invalid pin number '%s'\n", i+2, row[1])
			continue
		}

		gpioPins = append(gpioPins, GPIOPin{
			Name:     row[0],
			Pin:      pin,
			Function: row[2],
		})
	}
	return gpioPins, nil
}

func main() {
	soc := flag.String("soc", "", "SoC type (intel, qcom, or mtk)")
	csvFile := flag.String("csv", "", "Local file path for GPIO map CSV")
	outputFile := flag.String("output", "", "Output file path")
	flag.Parse()

	if *soc == "" {
		fmt.Fprintln(os.Stderr, "Error: --soc flag is required")
		flag.Usage()
		os.Exit(1)
	}

	if *csvFile == "" {
		fmt.Fprintln(os.Stderr, "Error: --csv flag is required")
		flag.Usage()
		os.Exit(1)
	}

	var outputWriter io.Writer
	if *outputFile != "" {
		f, err := os.Create(*outputFile)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error creating output file: %v\n", err)
			os.Exit(1)
		}
		defer f.Close()
		outputWriter = f
	} else {
		outputWriter = os.Stdout
	}

	// Open the local CSV file
	file, err := os.Open(*csvFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error opening CSV file: %v\n", err)
		os.Exit(1)
	}
	defer file.Close()

	fmt.Fprintf(os.Stderr, "SoC: %s\n", *soc)
	fmt.Fprintf(os.Stderr, "Reading GPIO map from: %s\n", *csvFile)

	reader := csv.NewReader(file)
	records, err := reader.ReadAll()
	if err != nil {
		fmt.Fprintf(os.Stderr, "Error parsing CSV: %v\n", err)
		os.Exit(1)
	}

	switch *soc {
	case "intel":
		err := processIntelGPIOs(records, outputWriter)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error processing Intel GPIOs: %v\n", err)
			os.Exit(1)
		}
	case "qcom", "mtk":
		gpioPins, err := parseGenericCSV(records)
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error parsing CSV: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintln(os.Stderr, "Successfully parsed GPIO pin data.")
		for _, pin := range gpioPins {
			fmt.Fprintf(outputWriter, "- Name: %s, Pin: %d, Function: %s\n", pin.Name, pin.Pin, pin.Function)
		}
	default:
		fmt.Fprintf(os.Stderr, "Error: Unsupported SoC '%s'. Must be one of 'intel', 'qcom', or 'mtk'.\n", *soc)
		flag.Usage()
		os.Exit(1)
	}

	if *outputFile != "" {
		fmt.Fprintf(os.Stderr, "Parsed data written to %s\n", *outputFile)
	}
}
