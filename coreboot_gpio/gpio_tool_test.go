// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coreboot_gpio

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestGpioTool_RunValidation(t *testing.T) {
	if err := Run([]string{"-h"}); err != nil {
		t.Errorf("Run('-h') unexpected error: %v", err)
	}
	if err := Run([]string{"--help"}); err != nil {
		t.Errorf("Run(--help) expected nil, got %v", err)
	}
	if err := Run([]string{}); err == nil {
		t.Errorf("expected error with no flags")
	}
	if err := Run([]string{"--soc=intel"}); err == nil {
		t.Errorf("expected error without --csv")
	}
	if err := Run([]string{"--soc=unsupported", "--csv=test/ExampleGpio.csv"}); err == nil {
		t.Errorf("expected error with unsupported SoC")
	}
}

func TestGpioTool_RunSuccess(t *testing.T) {
	out := filepath.Join(t.TempDir(), "gpio.c")
	err := Run([]string{
		"--soc=intel",
		"--csv=test/ExampleGpio.csv",
		"--output=" + out,
	})
	if err != nil {
		t.Fatalf("Run failed: %v", err)
	}

	data, err := os.ReadFile(out)
	if err != nil {
		t.Fatalf("failed to read output file: %v", err)
	}
	content := string(data)
	if !strings.Contains(content, "PAD_CFG") {
		t.Errorf("expected generated C file to contain PAD_CFG")
	}
}
