// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sheets

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"strings"
	"time"

	"firmweaver/shared/auth"
	"golang.org/x/oauth2"
	"google.golang.org/api/googleapi"
	"google.golang.org/api/option"
	sheetsapi "google.golang.org/api/sheets/v4"
)

const (
	DefaultSheetsAPIBase = "https://sheets.googleapis.com"
	APIBaseEnv           = "FIRMWEAVER_SHEETS_API_BASE"

	// fetchTimeout bounds the total time spent talking to the Sheets API.
	// The API client has no timeout of its own, so without this a stalled
	// connection would hang the tool indefinitely.
	fetchTimeout = 30 * time.Second
)

func getAPIBase() string {
	if env := os.Getenv(APIBaseEnv); env != "" {
		return env
	}
	return DefaultSheetsAPIBase
}

func newSheetsService(ctx context.Context, token string) (*sheetsapi.Service, error) {
	opts := []option.ClientOption{
		option.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: token,
			TokenType:   "Bearer",
		})),
	}
	if base := getAPIBase(); base != DefaultSheetsAPIBase {
		opts = append(opts, option.WithEndpoint(base))
	}
	return sheetsapi.NewService(ctx, opts...)
}

func formatSheetsAPIError(err error, contextMsg string) error {
	var gErr *googleapi.Error
	if errors.As(err, &gErr) {
		switch gErr.Code {
		case http.StatusUnauthorized:
			return fmt.Errorf("%s: OAuth token expired or invalid (HTTP 401). Run 'firmweaver auth login' to re-authenticate", contextMsg)
		case http.StatusForbidden:
			return fmt.Errorf("%s: permission denied (HTTP 403). Ensure your Google account has read access to this sheet", contextMsg)
		case http.StatusNotFound:
			return fmt.Errorf("%s: spreadsheet or tab not found (HTTP 404). Verify sheet URL or tab name", contextMsg)
		case http.StatusTooManyRequests:
			return fmt.Errorf("%s: Google Sheets API rate limit / quota exceeded (HTTP 429). Please retry later", contextMsg)
		default:
			return fmt.Errorf("%s: Google Sheets API returned HTTP %d: %s", contextMsg, gErr.Code, gErr.Message)
		}
	}
	return fmt.Errorf("%s: %w", contextMsg, err)
}

// Fetch resolves the sheet reference and returns the tab's cell matrix.
func Fetch(ctx context.Context, sheetRef, tabOverride string) ([][]string, error) {
	spreadsheetID, gid, err := ResolveSheetURL(ctx, sheetRef)
	if err != nil {
		return nil, err
	}
	return FetchSheetData(ctx, spreadsheetID, gid, tabOverride)
}

// FetchSheetData retrieves a Google Sheet tab and returns it as a 2D string matrix.
func FetchSheetData(ctx context.Context, spreadsheetID, gid, tabOverride string) ([][]string, error) {
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()

	token, _, err := auth.GetToken(ctx)
	if err != nil {
		return nil, fmt.Errorf("authentication error: %w", err)
	}

	srv, err := newSheetsService(ctx, token)
	if err != nil {
		return nil, fmt.Errorf("failed to create sheets service: %w", err)
	}

	tabTitle := tabOverride
	if tabTitle == "" {
		tabTitle, err = resolveTabTitle(ctx, srv, spreadsheetID, gid)
		if err != nil {
			return nil, err
		}
	}

	fmt.Fprintf(os.Stderr, "Reading Google Sheet tab %q...\n", tabTitle)

	escapedTab := "'" + strings.ReplaceAll(tabTitle, "'", "''") + "'"
	vr, err := srv.Spreadsheets.Values.Get(spreadsheetID, escapedTab).
		ValueRenderOption("FORMATTED_VALUE").
		Context(ctx).
		Do()
	if err != nil {
		return nil, formatSheetsAPIError(err, "fetching values")
	}

	if len(vr.Values) == 0 {
		return nil, fmt.Errorf("sheet contains no data rows")
	}

	maxCols := 0
	for _, row := range vr.Values {
		if len(row) > maxCols {
			maxCols = len(row)
		}
	}

	grid := make([][]string, len(vr.Values))
	for r, row := range vr.Values {
		grid[r] = make([]string, maxCols)
		for c := 0; c < maxCols; c++ {
			if c < len(row) && row[c] != nil {
				grid[r][c] = strings.TrimSpace(fmt.Sprint(row[c]))
			}
		}
	}
	return grid, nil
}

func resolveTabTitle(ctx context.Context, srv *sheetsapi.Service, spreadsheetID, gid string) (string, error) {
	resp, err := srv.Spreadsheets.Get(spreadsheetID).
		Fields("sheets(properties(sheetId,title))").
		Context(ctx).
		Do()
	if err != nil {
		return "", formatSheetsAPIError(err, "querying metadata")
	}
	if len(resp.Sheets) == 0 {
		return "", fmt.Errorf("spreadsheet contains no sheets/tabs")
	}

	if gid != "" {
		targetGID, err := strconv.ParseInt(gid, 10, 64)
		if err != nil {
			return "", fmt.Errorf("invalid tab gid %q: must be integer", gid)
		}
		for _, s := range resp.Sheets {
			if s.Properties != nil && s.Properties.SheetId == targetGID {
				return s.Properties.Title, nil
			}
		}
		return "", fmt.Errorf("sheet tab with gid=%s not found in spreadsheet", gid)
	}
	if resp.Sheets[0].Properties == nil {
		return "", fmt.Errorf("spreadsheet metadata is missing tab properties")
	}
	return resp.Sheets[0].Properties.Title, nil
}
