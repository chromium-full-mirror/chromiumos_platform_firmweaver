// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sheets

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"firmweaver/shared/auth"
)

const (
	googleSheetsBaseURL = "https://docs.google.com/spreadsheets/"
	mockSpreadsheetID   = "mock-test-spreadsheet-id-placeholder-12345"
)

func TestParseGoogleSheetsURL(t *testing.T) {
	ctx := context.Background()

	standardURL := googleSheetsBaseURL + "d/" + mockSpreadsheetID + "/edit#gid=0"
	id, gid, err := ResolveSheetURL(ctx, standardURL)
	if err != nil || id != mockSpreadsheetID || gid != "0" {
		t.Errorf("ResolveSheetURL standard URL failed: id=%s, gid=%s, err=%v", id, gid, err)
	}

	multiAccountURL := googleSheetsBaseURL + "u/1/d/" + mockSpreadsheetID + "/edit?gid=12345"
	id, gid, err = ResolveSheetURL(ctx, multiAccountURL)
	if err != nil || id != mockSpreadsheetID || gid != "12345" {
		t.Errorf("ResolveSheetURL multi-account URL failed: id=%s, gid=%s, err=%v", id, gid, err)
	}

	rawID := mockSpreadsheetID
	id, _, err = ResolveSheetURL(ctx, rawID)
	if err != nil || id != rawID {
		t.Errorf("ResolveSheetURL raw ID failed: id=%s, err=%v", id, err)
	}

	if _, _, err := ResolveSheetURL(ctx, "invalid-url"); err == nil {
		t.Errorf("ResolveSheetURL expected error for invalid URL")
	}
}

func TestFetchSheetDataMock(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/values/") {
			_ = json.NewEncoder(w).Encode(map[string]interface{}{
				"values": [][]interface{}{
					{"Header1", "Header2", "Header3"},
					{"Val1", "Val2"}, // Ragged row with missing 3rd column
				},
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

	t.Setenv(APIBaseEnv, server.URL)
	t.Setenv(auth.ConfigDirEnv, t.TempDir())

	_ = auth.SaveToken(&auth.Token{
		AccessToken: "mock-token",
		Expiry:      time.Now().Add(time.Hour),
	})

	testURL := googleSheetsBaseURL + "d/" + mockSpreadsheetID + "/edit#gid=0"
	grid, err := Fetch(context.Background(), testURL, "")
	if err != nil {
		t.Fatalf("Fetch() failed: %v", err)
	}
	if len(grid) != 2 || len(grid[1]) != 3 || grid[1][2] != "" {
		t.Errorf("expected padded grid with 3 columns, got: %+v", grid)
	}
}

func TestFetchSheetData_HTTPErrors(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "Forbidden", http.StatusForbidden)
	}))
	defer server.Close()

	t.Setenv(APIBaseEnv, server.URL)
	t.Setenv(auth.ConfigDirEnv, t.TempDir())

	_ = auth.SaveToken(&auth.Token{
		AccessToken: "mock-token",
		Expiry:      time.Now().Add(time.Hour),
	})

	testURL := googleSheetsBaseURL + "d/" + mockSpreadsheetID + "/edit"
	_, err := Fetch(context.Background(), testURL, "Tab1")
	if err == nil || !strings.Contains(err.Error(), "permission denied (HTTP 403)") {
		t.Errorf("expected permission denied error, got: %v", err)
	}
}

func TestFetchSheetData_MetadataMissingProperties(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Tab metadata without a "properties" object.
		_ = json.NewEncoder(w).Encode(map[string]interface{}{
			"sheets": []map[string]interface{}{{}},
		})
	}))
	defer server.Close()

	t.Setenv(APIBaseEnv, server.URL)
	t.Setenv(auth.ConfigDirEnv, t.TempDir())

	_ = auth.SaveToken(&auth.Token{
		AccessToken: "mock-token",
		Expiry:      time.Now().Add(time.Hour),
	})

	testURL := googleSheetsBaseURL + "d/" + mockSpreadsheetID + "/edit"
	if _, err := Fetch(context.Background(), testURL, ""); err == nil ||
		!strings.Contains(err.Error(), "missing tab properties") {
		t.Errorf("expected missing tab properties error, got: %v", err)
	}
}
