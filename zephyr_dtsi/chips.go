// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zephyr_dtsi

import (
	"embed"
	"encoding/json"
	"fmt"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

//go:embed data/chips/*.json
var embeddedChipsFS embed.FS

var (
	npcxRegex     = regexp.MustCompile(`GP[IO]{0,2}([0-9A-F])([0-7])`)
	iteRegex      = regexp.MustCompile(`GP([A-Z])([0-7])`)
	adcRegex      = regexp.MustCompile(`ADC([0-9]+)`)
	pwmRegex      = regexp.MustCompile(`PWM([0-9]+)`)
	npcxI2C       = regexp.MustCompile(`I2C([0-9]+)_(SCL|SDA)`)
	iteScl        = regexp.MustCompile(`SMCLK([0-9]+[A-Z]*)`)
	iteSda        = regexp.MustCompile(`SMDAT([0-9]+[A-Z]*)`)
	npcxTachRegex = regexp.MustCompile(`^T([A-Z])([0-9])$`)
	iteTachRegex  = regexp.MustCompile(`^TACH([0-9])([A-Z])$`)
)

type ChipData struct {
	Name    string            `json:"name"`
	Vendor  string            `json:"vendor"`
	Aliases []string          `json:"aliases"`
	PinMap  map[string]string `json:"pin_map"`
}

type Chip struct {
	Name    string
	Vendor  string
	Aliases []string
	PinMap  map[string]string
	PkgPins []string // List of sorted pin numbers (keys of PinMap)
}

func (c *Chip) IsNPCX() bool {
	if c.Vendor != "" {
		return strings.EqualFold(c.Vendor, "npcx")
	}
	return strings.HasPrefix(strings.ToLower(c.Name), "npcx")
}

func (c *Chip) IsITE() bool {
	if c.Vendor != "" {
		return strings.EqualFold(c.Vendor, "ite")
	}
	return strings.HasPrefix(strings.ToLower(c.Name), "it")
}

// ResolveGPIO returns the Zephyr GPIO port (e.g., "0", "e", "a") and pin number (0-7)
func (c *Chip) ResolveGPIO(pinNum string) (string, int, error) {
	pinDef, ok := c.PinMap[pinNum]
	if !ok {
		return "", 0, fmt.Errorf("pin %s not found in chip %s", pinNum, c.Name)
	}

	parts := strings.Split(pinDef, "/")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if m := npcxRegex.FindStringSubmatch(part); m != nil {
			port := strings.ToLower(m[1])
			pin, err := strconv.Atoi(m[2])
			if err != nil {
				return "", 0, fmt.Errorf("invalid GPIO pin number %q in %s: %v", m[2], pinDef, err)
			}
			return port, pin, nil
		}
	}

	for _, part := range parts {
		part = strings.TrimSpace(part)
		if m := iteRegex.FindStringSubmatch(part); m != nil {
			port := strings.ToLower(m[1])
			pin, err := strconv.Atoi(m[2])
			if err != nil {
				return "", 0, fmt.Errorf("invalid GPIO pin number %q in %s: %v", m[2], pinDef, err)
			}
			return port, pin, nil
		}
	}

	if c.IsITE() {
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if strings.HasPrefix(part, "KSO") {
				idxStr := strings.TrimPrefix(part, "KSO")
				idx, err := strconv.Atoi(idxStr)
				if err == nil {
					if idx >= 0 && idx <= 7 {
						return "ksol", idx, nil
					} else if idx >= 8 && idx <= 15 {
						return "ksoh", idx - 8, nil
					}
				}
			}
		}
	}

	return "", 0, fmt.Errorf("could not resolve GPIO for pin %s (%s)", pinNum, pinDef)
}

// ResolveADC returns the ADC channel (0-7) if the pin supports ADC
func (c *Chip) ResolveADC(pinNum string) (int, error) {
	pinDef, ok := c.PinMap[pinNum]
	if !ok {
		return 0, fmt.Errorf("pin %s not found in chip %s", pinNum, c.Name)
	}

	parts := strings.Split(pinDef, "/")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if m := adcRegex.FindStringSubmatch(part); m != nil {
			channel, err := strconv.Atoi(m[1])
			if err != nil {
				return 0, fmt.Errorf("invalid ADC channel %q in %s: %v", m[1], pinDef, err)
			}
			return channel, nil
		}
	}
	return 0, fmt.Errorf("pin %s (%s) does not support ADC", pinNum, pinDef)
}

// ResolvePWM returns the PWM channel if the pin supports PWM
func (c *Chip) ResolvePWM(pinNum string) (int, error) {
	pinDef, ok := c.PinMap[pinNum]
	if !ok {
		return 0, fmt.Errorf("pin %s not found in chip %s", pinNum, c.Name)
	}

	parts := strings.Split(pinDef, "/")
	for _, part := range parts {
		part = strings.TrimSpace(part)
		if m := pwmRegex.FindStringSubmatch(part); m != nil {
			channel, err := strconv.Atoi(m[1])
			if err != nil {
				return 0, fmt.Errorf("invalid PWM channel %q in %s: %v", m[1], pinDef, err)
			}
			return channel, nil
		}
	}
	return 0, fmt.Errorf("pin %s (%s) does not support PWM", pinNum, pinDef)
}

// ResolveI2C returns the I2C channel (as string, e.g. "0" or "2ALT") and whether it is SCL (true) or SDA (false)
func (c *Chip) ResolveI2C(pinNum string) (string, bool, error) {
	pinDef, ok := c.PinMap[pinNum]
	if !ok {
		return "", false, fmt.Errorf("pin %s not found in chip %s", pinNum, c.Name)
	}

	parts := strings.Split(pinDef, "/")
	for _, part := range parts {
		part = strings.TrimSpace(part)

		if m := npcxI2C.FindStringSubmatch(part); m != nil {
			isScl := m[2] == "SCL"
			return m[1], isScl, nil
		}

		if m := iteScl.FindStringSubmatch(part); m != nil {
			return m[1], true, nil
		}
		if m := iteSda.FindStringSubmatch(part); m != nil {
			return m[1], false, nil
		}
	}
	return "", false, fmt.Errorf("pin %s (%s) does not support I2C", pinNum, pinDef)
}

// ResolveTACH returns the TACH channel and port letter if the pin supports TACH.
func (c *Chip) ResolveTACH(pinNum string) (int, string, error) {
	pinDef, ok := c.PinMap[pinNum]
	if !ok {
		return 0, "", fmt.Errorf("pin %s not found in chip %s", pinNum, c.Name)
	}

	parts := strings.Split(pinDef, "/")
	if c.IsNPCX() {
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if m := npcxTachRegex.FindStringSubmatch(part); m != nil {
				portLetter := m[1]
				chanNum, err := strconv.Atoi(m[2])
				if err != nil {
					return 0, "", fmt.Errorf("invalid TACH channel %q in %s: %v", m[2], pinDef, err)
				}
				return chanNum, portLetter, nil
			}
		}
	} else if c.IsITE() {
		for _, part := range parts {
			part = strings.TrimSpace(part)
			if m := iteTachRegex.FindStringSubmatch(part); m != nil {
				chanNum, err := strconv.Atoi(m[1])
				if err != nil {
					return 0, "", fmt.Errorf("invalid TACH channel %q in %s: %v", m[1], pinDef, err)
				}
				portLetter := m[2]
				return chanNum, portLetter, nil
			}
		}
	}

	return 0, "", fmt.Errorf("pin %s (%s) does not support TACH", pinNum, pinDef)
}

var chips = map[string]*Chip{}

// GetChip returns the registered Chip by name or alias.
func GetChip(name string) (*Chip, bool) {
	chip, ok := chips[strings.ToLower(name)]
	return chip, ok
}

// GetAvailableChips returns a sorted, comma-separated list of all registered chip names and aliases.
func GetAvailableChips() string {
	names := slices.Collect(maps.Keys(chips))
	slices.Sort(names)
	return strings.Join(names, ", ")
}

func RegisterChipData(data []byte) error {
	var cd ChipData
	if err := json.Unmarshal(data, &cd); err != nil {
		return fmt.Errorf("failed to unmarshal chip json: %v", err)
	}

	var pins []string
	for k := range cd.PinMap {
		if k != "" {
			pins = append(pins, k)
		}
	}
	slices.Sort(pins)

	chip := &Chip{
		Name:    cd.Name,
		Vendor:  cd.Vendor,
		Aliases: cd.Aliases,
		PinMap:  cd.PinMap,
		PkgPins: pins,
	}

	chips[strings.ToLower(cd.Name)] = chip
	for _, alias := range cd.Aliases {
		chips[strings.ToLower(alias)] = chip
	}
	return nil
}

// LoadExternalChipDir loads custom chip definitions from an external directory
func LoadExternalChipDir(dirPath string) error {
	entries, err := os.ReadDir(dirPath)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !entry.IsDir() && strings.HasSuffix(entry.Name(), ".json") {
			fullPath := filepath.Join(dirPath, entry.Name())
			content, err := os.ReadFile(fullPath)
			if err != nil {
				return fmt.Errorf("failed to read chip file %s: %v", fullPath, err)
			}
			if err := RegisterChipData(content); err != nil {
				return fmt.Errorf("error loading chip file %s: %v", fullPath, err)
			}
		}
	}
	return nil
}

func init() {
	files, err := fs.ReadDir(embeddedChipsFS, "data/chips")
	if err != nil {
		panic(fmt.Sprintf("failed to read embedded chip dir: %v", err))
	}

	for _, file := range files {
		if strings.HasSuffix(file.Name(), ".json") {
			content, err := embeddedChipsFS.ReadFile(filepath.Join("data/chips", file.Name()))
			if err != nil {
				panic(fmt.Sprintf("failed to read embedded chip file %s: %v", file.Name(), err))
			}
			if err := RegisterChipData(content); err != nil {
				panic(fmt.Sprintf("failed to register embedded chip file %s: %v", file.Name(), err))
			}
		}
	}
}
