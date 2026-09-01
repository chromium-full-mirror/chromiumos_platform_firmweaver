// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zephyr_dtsi

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
	"firmweaver/shared/sheets"
)

func TestDtsiTool_RunFlagValidation(t *testing.T) {
	if err := Run([]string{"-h"}); err != nil {
		t.Errorf("Run('-h') unexpected error: %v", err)
	}
	if err := Run([]string{"--help"}); err != nil {
		t.Errorf("Run(--help) expected nil, got %v", err)
	}
	if err := Run([]string{"--chip=npcx99", "--csv=test/ExampleEcPinout.csv", "--sheet=go/rex-pins"}); err == nil {
		t.Errorf("expected error when both --csv and --sheet specified")
	}
	if err := Run([]string{"--chip=npcx99", "--csv=test/ExampleEcPinout.csv", "--sheet-tab=Sheet2"}); err == nil {
		t.Errorf("expected error when --sheet-tab specified with --csv")
	}
	if err := Run([]string{}); err == nil {
		t.Errorf("Run() expected error for missing flags, got nil")
	}
	if err := Run([]string{"--chip=npcx99"}); err == nil {
		t.Errorf("Run() expected error for missing --csv/--sheet, got nil")
	}
	if err := Run([]string{"--chip=invalid_chip", "--csv=test/ExampleEcPinout.csv"}); err == nil {
		t.Errorf("Run() expected error for invalid chip, got nil")
	}
}

func TestDtsiTool_RunSuccess(t *testing.T) {
	out := filepath.Join(t.TempDir(), "out.dtsi")
	err := Run([]string{
		"--chip=npcx99",
		"--pin-col=NPCX99 (NPCX)",
		"--csv=test/ExampleEcPinout.csv",
		"--output=" + out,
	})
	if err != nil {
		t.Fatalf("Run() failed: %v", err)
	}

	content, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	if !strings.Contains(string(content), "named-gpios") {
		t.Errorf("expected output to contain named-gpios, got: %s", string(content))
	}
}

const (
	googleSheetsBaseURL = "https://docs.google.com/spreadsheets/"
	mockSpreadsheetID   = "mock-test-spreadsheet-id-placeholder-12345"
)

func TestDtsiTool_RunSheetIntegration(t *testing.T) {
	t.Setenv(auth.ConfigDirEnv, t.TempDir())

	_ = auth.SaveToken(&auth.Token{
		AccessToken: "mock-sheet-token",
		Expiry:      time.Now().Add(time.Hour),
	})

	f, _ := os.Open("test/ExampleEcPinout.csv")
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

	out := filepath.Join(t.TempDir(), "sheet_out.dtsi")
	err := Run([]string{
		"--chip=npcx99",
		"--pin-col=NPCX99 (NPCX)",
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
	if !strings.Contains(string(data), "named-gpios") {
		t.Errorf("expected generated DTSI from sheet to contain named-gpios")
	}
}
