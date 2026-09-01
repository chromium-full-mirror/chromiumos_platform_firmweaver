// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zephyr_dtsi

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestDtsiTool_RunFlagValidation(t *testing.T) {
	if err := Run([]string{"-h"}); err != nil {
		t.Errorf("Run('-h') unexpected error: %v", err)
	}
	if err := Run([]string{"--help"}); err != nil {
		t.Errorf("Run(--help) expected nil, got %v", err)
	}
	if err := Run([]string{}); err == nil {
		t.Errorf("expected error with no flags")
	}
	if err := Run([]string{"--chip=npcx99"}); err == nil {
		t.Errorf("expected error without --csv")
	}
	if err := Run([]string{"--chip=invalid", "--csv=test/ExampleEcPinout.csv"}); err == nil {
		t.Errorf("expected error with invalid chip")
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
		t.Fatalf("Run failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("failed to read generated file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "named-gpios") {
		t.Errorf("expected generated DTSI to contain named-gpios")
	}
}
