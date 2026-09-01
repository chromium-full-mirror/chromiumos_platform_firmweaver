// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"
	"encoding/json"
	"fmt"
	"math/rand/v2"
	"os"
	"time"

	"firmweaver/shared/auth"
	"golang.org/x/oauth2"
	bigquery "google.golang.org/api/bigquery/v2"
	"google.golang.org/api/option"
)

// DefaultEndpoint is the BigQuery service metrics rows are streamed to.
const DefaultEndpoint = "https://bigquery.googleapis.com"

// Endpoint allows overriding the destination service in unit tests.
var Endpoint = DefaultEndpoint

// Destination table for usage metrics. The client escapes the project ID,
// so the "google.com:" prefix is written literally here.
const (
	projectID = "google.com:chromeos-arbitrage"
	datasetID = "firmweaver"
	tableID   = "user_logs"
)

const (
	DisableMetricsEnv = "FIRMWEAVER_DISABLE_METRICS"
	// DoNotTrackEnv adheres to the Console Do Not Track standard (https://consoledonottrack.com).
	DoNotTrackEnv = "DO_NOT_TRACK"
)

// Event captures an execution log row matching the BigQuery metrics schema.
type Event struct {
	EventID       string   `json:"event_id"`
	TimestampUTC  string   `json:"timestamp_utc"`
	UserLDAP      string   `json:"user_ldap"`
	ToolName      string   `json:"tool_name"`
	Subcommand    string   `json:"subcommand"`
	TargetChip    string   `json:"target_chip,omitempty"`
	TargetSoC     string   `json:"target_soc,omitempty"`
	VariantHeader string   `json:"variant_header,omitempty"`
	InputMode     string   `json:"input_mode,omitempty"`
	Flags         []string `json:"flags,omitempty"`
	DurationMs    int64    `json:"duration_ms"`
	Success       bool     `json:"success"`
	ErrorMsg      string   `json:"error_msg,omitempty"`
}

// IsDisabled checks if metrics collection is explicitly opted out.
func IsDisabled() bool {
	return os.Getenv(DoNotTrackEnv) == "1" || os.Getenv(DisableMetricsEnv) == "1"
}

// NewEvent initializes a new Event with timestamp and user.
func NewEvent(toolName, subcommand string) *Event {
	ldap := os.Getenv("USER")
	if ldap == "" {
		ldap = os.Getenv("LOGNAME")
	}
	if ldap == "" {
		ldap = "unknown"
	}

	return &Event{
		EventID:      fmt.Sprintf("%s-%d-%08x", toolName, time.Now().UnixNano(), rand.Uint32()),
		TimestampUTC: time.Now().UTC().Format(time.RFC3339),
		UserLDAP:     ldap,
		ToolName:     toolName,
		Subcommand:   subcommand,
	}
}

// eventRow renders the event using its json tags so the row keys line up with
// the BigQuery table schema.
func eventRow(ev *Event) (map[string]bigquery.JsonValue, error) {
	raw, err := json.Marshal(ev)
	if err != nil {
		return nil, err
	}
	var row map[string]bigquery.JsonValue
	if err := json.Unmarshal(raw, &row); err != nil {
		return nil, err
	}
	return row, nil
}

// Send silently flushes the event to BigQuery with a bounded 1-second timeout.
func Send(ev *Event) {
	if ev == nil || IsDisabled() {
		return
	}

	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()

	token, _, err := auth.GetToken(ctx)
	if err != nil || token == "" {
		return
	}

	row, err := eventRow(ev)
	if err != nil {
		return
	}

	opts := []option.ClientOption{
		option.WithTokenSource(oauth2.StaticTokenSource(&oauth2.Token{
			AccessToken: token,
			TokenType:   "Bearer",
		})),
	}
	if Endpoint != DefaultEndpoint {
		opts = append(opts, option.WithEndpoint(Endpoint))
	}

	srv, err := bigquery.NewService(ctx, opts...)
	if err != nil {
		return
	}

	_, _ = srv.Tabledata.InsertAll(projectID, datasetID, tableID, &bigquery.TableDataInsertAllRequest{
		Kind:                "bigquery#tableDataInsertAllRequest",
		IgnoreUnknownValues: true,
		Rows: []*bigquery.TableDataInsertAllRequestRows{{
			InsertId: ev.EventID,
			Json:     row,
		}},
	}).Context(ctx).Do()
}
