// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"slices"
	"strings"
	"testing"
)

func TestGetAvailableChips(t *testing.T) {
	chipsStr := GetAvailableChips()
	chips := strings.Split(chipsStr, ", ")

	if !slices.IsSorted(chips) {
		t.Errorf("GetAvailableChips() returned unsorted list: %q", chipsStr)
	}

	expected := []string{
		"it81202", "it81302", "it82002", "it82202", "it82202ax",
		"npcx99", "npcx993", "npcx996", "npcx99*",
	}
	for _, name := range expected {
		if !slices.Contains(chips, name) {
			t.Errorf("Expected chip %q to be in available chips list: %q", name, chipsStr)
		}
	}
}
