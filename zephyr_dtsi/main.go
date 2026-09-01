// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"flag"
	"fmt"
	"io"
	"os"
	"path/filepath"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintf(os.Stderr, "%v\n", err)
		os.Exit(1)
	}
}

func run() error {
	chipFlag := flag.String("chip", "", "Supported EC chip name or alias (e.g. npcx99, npcx993, it81302, it81202, it82002, it82202)")
	csvFlag := flag.String("csv", "", "Path to the EC pinout CSV file")
	pinColFlag := flag.String("pin-col", "", "Optional: Column header containing pin numbers (e.g. 'NPCX99 (NPCX)' or 'IT82002 (ITE)')")
	chipDataDirFlag := flag.String("chip-data-dir", "", "Optional: Path to directory containing custom chip JSON data files")
	outputFlag := flag.String("output", "", "Path to the output DTSI file (defaults to stdout)")
	flag.Parse()

	if *chipDataDirFlag != "" {
		if err := LoadExternalChipDir(*chipDataDirFlag); err != nil {
			return fmt.Errorf("error loading chip data dir %s: %w", *chipDataDirFlag, err)
		}
	}

	if *chipFlag == "" {
		flag.Usage()
		return fmt.Errorf("error: --chip is required. Available chips: %s", GetAvailableChips())
	}

	if *csvFlag == "" {
		flag.Usage()
		return fmt.Errorf("error: --csv is required")
	}

	chip, ok := GetChip(*chipFlag)
	if !ok {
		return fmt.Errorf("error: Unsupported chip %q. Available: %s", *chipFlag, GetAvailableChips())
	}

	csvFile, err := os.Open(*csvFlag)
	if err != nil {
		return fmt.Errorf("error opening CSV file: %w", err)
	}
	defer csvFile.Close()

	configs, err := ParseCSV(csvFile, *pinColFlag)
	if err != nil {
		return fmt.Errorf("error parsing CSV: %w", err)
	}

	var out io.Writer = os.Stdout
	var tmpFile *os.File
	if *outputFlag != "" {
		dir := filepath.Dir(*outputFlag)
		var err error
		tmpFile, err = os.CreateTemp(dir, ".tmp_generated_*.dtsi")
		if err != nil {
			return fmt.Errorf("error creating temp file: %w", err)
		}
		defer func() {
			tmpFile.Close()
			os.Remove(tmpFile.Name())
		}()
		out = tmpFile
	}

	if err := GenerateDTSI(out, configs, chip); err != nil {
		return fmt.Errorf("error generating DTSI: %w", err)
	}

	if tmpFile != nil {
		tmpFile.Close()
		if err := os.Rename(tmpFile.Name(), *outputFlag); err != nil {
			return fmt.Errorf("error saving output file: %w", err)
		}
	}

	return nil
}
