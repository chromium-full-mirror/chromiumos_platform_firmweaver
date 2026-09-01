// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coreboot_gpio

import (
	"encoding/csv"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"firmweaver/shared/auth"
	"firmweaver/shared/metrics"
	"firmweaver/shared/sheets"
)

func TestGpioTool_RunValidation(t *testing.T) {
	t.Setenv(metrics.DisableMetricsEnv, "1")
	if err := Run([]string{"-h"}); err != nil {
		t.Errorf("Run('-h') unexpected error: %v", err)
	}
	if err := Run([]string{"--help"}); err != nil {
		t.Errorf("Run('--help') unexpected error: %v", err)
	}
	if err := Run([]string{"--soc=intel", "--csv=test/ExampleGpio.csv", "--sheet=go/rex-pins"}); err == nil {
		t.Errorf("expected error when both --csv and --sheet specified")
	}
	if err := Run([]string{"--soc=intel", "--csv=test/ExampleGpio.csv", "--sheet-tab=Sheet2"}); err == nil {
		t.Errorf("expected error when --sheet-tab specified with --csv")
	}
	if err := Run([]string{}); err == nil {
		t.Errorf("Run() expected error for missing flags, got nil")
	}
	if err := Run([]string{"--soc=intel"}); err == nil {
		t.Errorf("Run() expected error for missing --csv/--sheet, got nil")
	}
	if err := Run([]string{"--soc=unsupported_soc", "--csv=test/ExampleGpio.csv"}); err == nil {
		t.Errorf("Run() expected error for unsupported SoC, got nil")
	}
}

func TestGpioTool_RunSuccess(t *testing.T) {
	t.Setenv(metrics.DisableMetricsEnv, "1")
	out := filepath.Join(t.TempDir(), "gpio.c")
	err := Run([]string{
		"--soc=intel",
		"--csv=test/ExampleGpio.csv",
		"--output=" + out,
	})
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if !strings.Contains(string(content), "PAD_CFG") {
		t.Errorf("expected output to contain PAD_CFG, got: %s", string(content))
	}
}

const (
	googleSheetsBaseURL = "https://docs.google.com/spreadsheets/"
	mockSpreadsheetID   = "mock-test-spreadsheet-id-placeholder-12345"
)

func TestGpioTool_RunSheetIntegration(t *testing.T) {
	t.Setenv(metrics.DisableMetricsEnv, "1")
	t.Setenv(auth.ConfigDirEnv, t.TempDir())

	_ = auth.SaveToken(&auth.Token{
		AccessToken: "mock-sheet-token",
		Expiry:      time.Now().Add(time.Hour),
	})

	f, _ := os.Open("test/ExampleGpio.csv")
	defer f.Close()
	r := csv.NewReader(f)
	r.FieldsPerRecord = -1
	csvRows, _ := r.ReadAll()
	var sheetValues [][]interface{}
	for i := 0; i < len(csvRows); i++ {
		var row []interface{}
		for _, cell := range csvRows[i] {
			row = append(row, cell)
		}
		sheetValues = append(sheetValues, row)
	}

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/values/") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"values": sheetValues,
			})
			return
		}
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"sheets": []map[string]interface{}{
				{
					"properties": map[string]interface{}{
						"sheetId": 0,
						"title":   "Sheet1",
					},
				},
			},
		})
	}))
	defer server.Close()

	t.Setenv(sheets.APIBaseEnv, server.URL)

	out := filepath.Join(t.TempDir(), "sheet_gpio.c")
	err := Run([]string{
		"--soc=intel",
		"--sheet=" + googleSheetsBaseURL + "d/" + mockSpreadsheetID + "/edit#gid=0",
		"--output=" + out,
	})
	if err != nil {
		t.Fatalf("Run(--sheet) failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if !strings.Contains(string(data), "PAD_CFG") {
		t.Errorf("expected generated C file from sheet to contain PAD_CFG")
	}
}
