// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zephyr_dtsi

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"

	"firmweaver/shared/sheets"
)

// Run executes the EC firmware pinmux DTSI generation logic.
func Run(args []string) error {
	fs := flag.NewFlagSet("zephyr_dtsi", flag.ContinueOnError)
	chipFlag := fs.String("chip", "", "Supported EC chip name or alias (e.g. npcx99, it82002)")
	csvFlag := fs.String("csv", "", "Path to the local EC pinout CSV file")
	sheetFlag := fs.String("sheet", "", "Google Sheets URL, ID, or go/ shortlink")
	sheetTabFlag := fs.String("sheet-tab", "", "Optional: Specific Google Sheet tab title to read")
	pinColFlag := fs.String("pin-col", "", "Optional: Column header containing pin numbers")
	chipDataDirFlag := fs.String("chip-data-dir", "", "Optional: Path to directory containing custom chip JSON data files")
	outputFlag := fs.String("output", "", "Path to the output DTSI file (defaults to stdout)")

	if err := fs.Parse(args); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	}
	if len(fs.Args()) > 0 {
		return fmt.Errorf("unexpected arguments: %v", fs.Args())
	}

	if *chipDataDirFlag != "" {
		if err := LoadExternalChipDir(*chipDataDirFlag); err != nil {
			return fmt.Errorf("loading chip data dir %s: %w", *chipDataDirFlag, err)
		}
	}

	if *chipFlag == "" {
		return fmt.Errorf("--chip is required (available: %s)", GetAvailableChips())
	}
	if *csvFlag != "" && *sheetFlag != "" {
		return fmt.Errorf("cannot specify both --csv and --sheet")
	}
	if *csvFlag == "" && *sheetFlag == "" {
		return fmt.Errorf("either --csv or --sheet must be provided")
	}
	if *sheetTabFlag != "" && *sheetFlag == "" {
		return fmt.Errorf("--sheet-tab can only be used with --sheet")
	}

	chip, ok := GetChip(*chipFlag)
	if !ok {
		return fmt.Errorf("unsupported chip %q (available: %s)", *chipFlag, GetAvailableChips())
	}

	var configs []PinConfig
	var err error
	if *sheetFlag != "" {
		records, fetchErr := sheets.Fetch(context.Background(), *sheetFlag, *sheetTabFlag)
		if fetchErr != nil {
			return fmt.Errorf("fetching sheet data: %w", fetchErr)
		}
		configs, err = ParseRecords(records, *pinColFlag)
		if err != nil {
			return fmt.Errorf("parsing sheet records: %w", err)
		}
	} else {
		csvFile, openErr := os.Open(*csvFlag)
		if openErr != nil {
			return fmt.Errorf("opening CSV file: %w", openErr)
		}
		defer csvFile.Close()

		configs, err = ParseCSV(csvFile, *pinColFlag)
		if err != nil {
			return fmt.Errorf("parsing CSV: %w", err)
		}
	}

	var out io.Writer = os.Stdout
	var tmpFile *os.File
	var tmpPath string
	if *outputFlag != "" {
		dir := filepath.Dir(*outputFlag)
		if err := os.MkdirAll(dir, 0755); err != nil {
			return fmt.Errorf("creating output directory: %w", err)
		}
		tmp, err := os.CreateTemp(dir, ".tmp_dtsi_*")
		if err != nil {
			return fmt.Errorf("creating temporary file: %w", err)
		}
		tmpFile = tmp
		tmpPath = tmp.Name()
		out = tmp
	}
	defer func() {
		if tmpFile != nil {
			_ = tmpFile.Close()
			if tmpPath != "" {
				_ = os.Remove(tmpPath)
			}
		}
	}()

	if err := GenerateDTSI(out, configs, chip); err != nil {
		return fmt.Errorf("generating DTSI: %w", err)
	}

	if tmpFile != nil {
		if err := tmpFile.Close(); err != nil {
			return fmt.Errorf("closing output file: %w", err)
		}
		if err := os.Rename(tmpPath, *outputFlag); err != nil {
			return fmt.Errorf("finalizing output file: %w", err)
		}
		tmpFile = nil
	}

	return nil
}
