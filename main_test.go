// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"testing"
)

func TestRun_SubcommandRouting(t *testing.T) {
	if err := run([]string{}); err == nil {
		t.Error("run([]) expected error for missing subcommand, got nil")
	}
	if err := run([]string{"help"}); err != nil {
		t.Errorf("run('help') unexpected error: %v", err)
	}
	if err := run([]string{"-h"}); err != nil {
		t.Errorf("run('-h') unexpected error: %v", err)
	}
	if err := run([]string{"--help"}); err != nil {
		t.Errorf("run('--help') unexpected error: %v", err)
	}
	if err := run([]string{"zephyr_dtsi", "-h"}); err != nil {
		t.Errorf("run('zephyr_dtsi -h') unexpected error: %v", err)
	}
	if err := run([]string{"zephyr_dtsi", "--help"}); err != nil {
		t.Errorf("run('zephyr_dtsi --help') unexpected error: %v", err)
	}
	if err := run([]string{"dtsi", "-h"}); err != nil {
		t.Errorf("run('dtsi -h') unexpected error: %v", err)
	}
	if err := run([]string{"coreboot_gpio", "-h"}); err != nil {
		t.Errorf("run('coreboot_gpio -h') unexpected error: %v", err)
	}
	if err := run([]string{"coreboot_gpio", "--help"}); err != nil {
		t.Errorf("run('coreboot_gpio --help') unexpected error: %v", err)
	}
	if err := run([]string{"gpio", "-h"}); err != nil {
		t.Errorf("run('gpio -h') unexpected error: %v", err)
	}
	if err := run([]string{"invalid"}); err == nil {
		t.Error("run('invalid') expected error for unknown command, got nil")
	}
}
