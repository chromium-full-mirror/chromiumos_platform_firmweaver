// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"firmweaver/shared/auth"
)

// wireRequest mirrors the BigQuery insertAll payload so the test can assert on
// exactly what goes over the wire.
type wireRequest struct {
	Kind                string    `json:"kind"`
	IgnoreUnknownValues bool      `json:"ignoreUnknownValues"`
	Rows                []wireRow `json:"rows"`
}

type wireRow struct {
	InsertID string `json:"insertId"`
	JSON     *Event `json:"json"`
}

func TestMetrics_SendAndPayload(t *testing.T) {
	t.Setenv(auth.ConfigDirEnv, t.TempDir())
	t.Setenv(DisableMetricsEnv, "")
	t.Setenv(DoNotTrackEnv, "")

	_ = auth.SaveToken(&auth.Token{
		AccessToken: "mock-token",
		Expiry:      time.Now().Add(time.Hour),
		Email:       "engineer@google.com",
	})

	var receivedReq wireRequest
	var authHeader string
	var requestPath string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		authHeader = r.Header.Get("Authorization")
		requestPath = r.URL.Path
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &receivedReq)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{}`))
	}))
	defer server.Close()

	origEndpoint := Endpoint
	Endpoint = server.URL
	t.Cleanup(func() { Endpoint = origEndpoint })

	ev := NewEvent("zephyr_dtsi", "generate")
	ev.TargetChip = "npcx99"
	ev.Flags = []string{"--chip=npcx99", "--csv=pins.csv"}
	ev.DurationMs = 120
	ev.Success = true

	Send(ev)

	if authHeader != "Bearer mock-token" {
		t.Errorf("expected Authorization header 'Bearer mock-token', got %q", authHeader)
	}
	wantPath := "/projects/google.com:chromeos-arbitrage/datasets/firmweaver/tables/user_logs/insertAll"
	if requestPath != wantPath {
		t.Errorf("request path = %q, want %q", requestPath, wantPath)
	}
	if len(receivedReq.Rows) != 1 {
		t.Fatalf("expected 1 row, got %d", len(receivedReq.Rows))
	}
	row := receivedReq.Rows[0]
	if row.InsertID != ev.EventID {
		t.Errorf("insertId = %q, want %q", row.InsertID, ev.EventID)
	}
	if row.JSON.ToolName != "zephyr_dtsi" || row.JSON.TargetChip != "npcx99" || !row.JSON.Success {
		t.Errorf("unexpected event payload: %+v", row.JSON)
	}
}

func TestMetrics_UnauthenticatedShortCircuit(t *testing.T) {
	t.Setenv(auth.ConfigDirEnv, t.TempDir()) // empty dir, no token
	t.Setenv(DisableMetricsEnv, "")
	t.Setenv(DoNotTrackEnv, "")

	called := false
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		called = true
	}))
	defer server.Close()

	origEndpoint := Endpoint
	Endpoint = server.URL
	t.Cleanup(func() { Endpoint = origEndpoint })

	ev := NewEvent("coreboot_gpio", "generate")
	Send(ev)

	if called {
		t.Errorf("Send() should not dial backend when user is unauthenticated")
	}
}

func TestMetrics_OptOut(t *testing.T) {
	t.Setenv(auth.ConfigDirEnv, t.TempDir())

	tests := []struct {
		envVar string
		want   bool
	}{
		{DisableMetricsEnv, true},
		{DoNotTrackEnv, true},
	}

	for _, tc := range tests {
		t.Run(tc.envVar, func(t *testing.T) {
			t.Setenv(DisableMetricsEnv, "")
			t.Setenv(DoNotTrackEnv, "")

			t.Setenv(tc.envVar, "1")
			if !IsDisabled() {
				t.Errorf("IsDisabled() with %s=1 should be true", tc.envVar)
			}
		})
	}

	t.Run("EnabledWhenUnset", func(t *testing.T) {
		t.Setenv(DisableMetricsEnv, "")
		t.Setenv(DoNotTrackEnv, "")

		if IsDisabled() {
			t.Errorf("IsDisabled() when env vars unset should be false")
		}
	})
}
