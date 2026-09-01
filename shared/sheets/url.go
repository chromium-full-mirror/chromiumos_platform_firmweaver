// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package sheets

import (
	"context"
	"fmt"
	"net/http"
	"net/url"
	"os/exec"
	"regexp"
	"strings"
	"time"
)

var sheetIDRegex = regexp.MustCompile(`^[a-zA-Z0-9_-]{25,}$`)

// ResolveSheetURL extracts spreadsheet ID and optional tab GID from a URL, go/ shortlink, or raw ID.
func ResolveSheetURL(ctx context.Context, input string) (string, string, error) {
	input = strings.TrimSpace(input)
	if input == "" {
		return "", "", fmt.Errorf("sheet input cannot be empty")
	}

	// 1. Resolve internal go/ shortlinks
	clean := strings.TrimPrefix(strings.TrimPrefix(input, "https://"), "http://")
	if strings.HasPrefix(strings.ToLower(clean), "go/") {
		if out, err := exec.CommandContext(ctx, "/google/bin/releases/golinks/goctl/goctl.par", "resolve", clean).Output(); err == nil {
			if res := strings.TrimSpace(string(out)); strings.HasPrefix(res, "http") {
				input = res
			}
		} else {
			client := &http.Client{Timeout: 5 * time.Second}
			req, reqErr := http.NewRequestWithContext(ctx, http.MethodHead, "http://"+clean, nil)
			if reqErr == nil {
				if resp, err := client.Do(req); err == nil {
					_ = resp.Body.Close()
					input = resp.Request.URL.String()
				}
			}
		}
	}

	// 2. Raw spreadsheet ID
	if !strings.Contains(input, "/") {
		if sheetIDRegex.MatchString(input) {
			return input, "", nil
		}
		return "", "", fmt.Errorf("invalid Google Sheet URL or ID %q", input)
	}

	// 3. Extract spreadsheet ID and GID from URL
	u, err := url.Parse(input)
	if err != nil {
		return "", "", fmt.Errorf("invalid sheet URL: %w", err)
	}

	parts := strings.Split(u.Path, "/d/")
	if len(parts) < 2 {
		return "", "", fmt.Errorf("invalid Google Sheet URL %q: missing /d/<id>", input)
	}
	sheetID := strings.Split(parts[1], "/")[0]
	if sheetID == "" {
		return "", "", fmt.Errorf("invalid Google Sheet URL %q: empty spreadsheet ID", input)
	}

	gid := u.Query().Get("gid")
	if gid == "" && strings.Contains(u.Fragment, "gid=") {
		for _, param := range strings.Split(u.Fragment, "&") {
			if strings.HasPrefix(param, "gid=") {
				gid = strings.TrimPrefix(param, "gid=")
				if idx := strings.IndexAny(gid, "?#& "); idx != -1 {
					gid = gid[:idx]
				}
				break
			}
		}
	}

	return sheetID, gid, nil
}
