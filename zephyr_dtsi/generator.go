// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zephyr_dtsi

import (
	"cmp"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strconv"
	"strings"
)

var (
	digitsRegex       = regexp.MustCompile(`[0-9]+`)
	npcxSclPinRegex   = regexp.MustCompile(`I2C([0-9]+)_SCL([0-9]+)`)
	npcxI2cIndexRegex = regexp.MustCompile(`I2C[0-9]+_(SCL|SDA)([0-9]+)`)
	netNameReplacer   = strings.NewReplacer(" ", "", "/", "_", "#", "_L")
)

type gpioPin struct {
	label  string
	pin    string
	port   string
	pinNum int
	flags  string
	enum   string
}

type unusedPin struct {
	comment string
	port    string
	pinNum  int
	flags   string
}

type adcPin struct {
	label   string
	pin     string
	channel int
	enum    string
}

type i2cPort struct {
	label    string
	pin      string
	channel  string
	portNode string
	enums    []string
}

type iteUartNode struct {
	rxGP string
	txGP string
}

// dtsiGenerator holds the state for generating the DTSI file.
type dtsiGenerator struct {
	w           io.Writer
	chip        *Chip
	gpios       []gpioPin
	unusedPins  []unusedPin
	adcs        []adcPin
	i2cPorts    []i2cPort
	i2cEnables  map[string][]string
	i2cCtrls    map[string]bool
	pwmEnables  map[int]string
	adcEnabled  bool
	adcPinctrls []string
	tachEnables map[int]string
	tachPorts   map[int]string
	iteUarts    map[int]iteUartNode
	kbPins      []string
	kbSleepPins []string
	maxKsi      int
	maxKso      int
}

func newDTSIGenerator(w io.Writer, chip *Chip) *dtsiGenerator {
	return &dtsiGenerator{
		w:           w,
		chip:        chip,
		i2cEnables:  make(map[string][]string),
		i2cCtrls:    make(map[string]bool),
		pwmEnables:  make(map[int]string),
		tachEnables: make(map[int]string),
		tachPorts:   make(map[int]string),
		iteUarts:    make(map[int]iteUartNode),
		maxKsi:      -1,
		maxKso:      -1,
	}
}

// GenerateDTSI generates the Zephyr DTSI content and writes it to w.
func GenerateDTSI(w io.Writer, configs []PinConfig, chip *Chip) error {
	g := newDTSIGenerator(w, chip)
	processedPins := make(map[string]bool)

	for _, cfg := range configs {
		if processedPins[cfg.PinNumber] {
			continue
		}
		processedPins[cfg.PinNumber] = true

		if cfg.NetName == "" && cfg.PinTypeOverride != "UNUSED_IN" && cfg.PinTypeOverride != "UNUSED_OUT" {
			continue
		}

		netName := cfg.NetName
		if cfg.NetNameOverride != "" {
			netName = cfg.NetNameOverride
		}
		netName = cleanNetName(netName)

		// Check excluded net names
		if strings.HasPrefix(netName, "ESPI_") || strings.HasSuffix(netName, "EC_RST_ODL") {
			continue
		}

		if cfg.PinTypeOverride == "OTHER" {
			continue
		}

		port, pinNum, gpioErr := chip.ResolveGPIO(cfg.PinNumber)
		gpName := ""
		if gpioErr == nil {
			gpName = fmt.Sprintf("gp%s%d", port, pinNum)
		}

		if cfg.PinTypeOverride == "UNUSED_IN" || cfg.PinTypeOverride == "UNUSED_OUT" {
			if gpioErr != nil {
				if def, ok := chip.PinMap[cfg.PinNumber]; ok {
					fmt.Fprintf(os.Stderr, "Warning: skipping %s: pin %s (%s) has no GPIO function\n", netName, cfg.PinNumber, def)
					continue
				}
				return fmt.Errorf("failed to resolve GPIO for pin %s: %v", cfg.PinNumber, gpioErr)
			}
			g.processUnusedPin(cfg, netName, port, pinNum)
			continue
		}

		// Try processing as Keyboard matrix
		if g.tryProcessKeyboard(cfg, netName, gpName) {
			continue
		}

		// Try processing as Tachometer
		processedTach, err := g.tryProcessTach(cfg, netName, gpName)
		if err != nil {
			return err
		}
		if processedTach {
			continue
		}

		// Try processing as ADC
		processedADC, err := g.tryProcessADC(cfg, netName, gpName)
		if err != nil {
			return err
		}
		if processedADC {
			continue
		}

		// Try processing as PWM
		processedPWM, err := g.tryProcessPWM(cfg, port, pinNum, gpName)
		if err != nil {
			return err
		}
		if processedPWM {
			continue
		}

		// Try processing as I2C
		processedI2C, err := g.tryProcessI2C(cfg, netName, port, pinNum, gpName)
		if err != nil {
			return err
		}
		if processedI2C {
			continue
		}

		// Try processing as UART
		processedUART, err := g.tryProcessUART(cfg, netName, gpName)
		if err != nil {
			return err
		}
		if processedUART || strings.HasPrefix(netName, "UART_") {
			continue
		}

		// Process as regular GPIO
		if isGPIO(cfg) {
			if gpioErr != nil {
				if def, ok := chip.PinMap[cfg.PinNumber]; ok {
					fmt.Fprintf(os.Stderr, "Warning: skipping %s: pin %s (%s) has no GPIO function\n", netName, cfg.PinNumber, def)
					continue
				}
				return fmt.Errorf("failed to resolve GPIO for pin %s: %v", cfg.PinNumber, gpioErr)
			}
			g.processGPIO(cfg, netName, port, pinNum, gpName)
		}
	}

	return g.write()
}

func (g *dtsiGenerator) tryProcessKeyboard(cfg PinConfig, netName, gpName string) bool {
	isKB := cfg.PinTypeOverride == "KEYBOARD"
	if !isKB {
		return false
	}

	prefix := "kso"
	if strings.Contains(netName, "KSI") {
		prefix = "ksi"
	}
	idx := findDigits(netName)
	if idx == "" {
		return false
	}

	idxNum, err := strconv.Atoi(idx)
	if err != nil {
		return false
	}

	if prefix == "ksi" {
		if idxNum > g.maxKsi {
			g.maxKsi = idxNum
		}
	} else {
		if idxNum > g.maxKso {
			g.maxKso = idxNum
		}
	}

	if len(idx) > 1 && idx[0] == '0' {
		if prefix == "ksi" || !g.chip.IsNPCX() {
			idx = idx[1:]
		}
	}

	if g.chip.IsNPCX() {
		if gpName != "" {
			g.kbPins = append(g.kbPins, fmt.Sprintf("&%s%s_%s", prefix, idx, gpName))
			g.kbSleepPins = append(g.kbSleepPins, fmt.Sprintf("&%s%s_gpio_%s", prefix, idx, gpName))
		}
	} else {
		g.kbPins = append(g.kbPins, fmt.Sprintf("&%s%s_default", prefix, idx))
		g.kbSleepPins = append(g.kbSleepPins, fmt.Sprintf("&%s%s_sleep", prefix, idx))
	}
	return true
}

func (g *dtsiGenerator) tryProcessTach(cfg PinConfig, netName, gpName string) (bool, error) {
	isTach := cfg.PinTypeOverride == "TACH" || strings.HasSuffix(netName, "_TACH")
	if !isTach {
		return false, nil
	}

	chanNum, portLetter, err := g.chip.ResolveTACH(cfg.PinNumber)
	if err != nil {
		return false, fmt.Errorf("failed to resolve TACH for pin %s: %w", cfg.PinNumber, err)
	}

	if g.chip.IsNPCX() {
		pinctrl := fmt.Sprintf("&t%s%d_1_in_%s", strings.ToLower(portLetter), chanNum, gpName)
		g.tachEnables[chanNum] = pinctrl
		g.tachPorts[chanNum] = portLetter
	} else {
		// ITE
		pinctrl := fmt.Sprintf("&tach%d%s_%s_default", chanNum, strings.ToLower(portLetter), gpName)
		g.tachEnables[chanNum] = pinctrl
		g.tachPorts[chanNum] = portLetter
	}
	return true, nil
}

func (g *dtsiGenerator) tryProcessADC(cfg PinConfig, netName, gpName string) (bool, error) {
	isADC := cfg.IO == "ADC" || cfg.PinTypeOverride == "ADC"
	if !isADC {
		return false, nil
	}

	adcChan, err := g.chip.ResolveADC(cfg.PinNumber)
	if err != nil {
		return false, fmt.Errorf("failed to resolve ADC for pin %s: %v", cfg.PinNumber, err)
	}
	g.adcEnabled = true
	label := fmt.Sprintf("adc_%s", strings.ToLower(netName))
	g.adcs = append(g.adcs, adcPin{
		label:   label,
		pin:     strings.ToLower(netName),
		channel: adcChan,
		enum:    cfg.Enum,
	})

	if gpName != "" {
		if g.chip.IsNPCX() {
			g.adcPinctrls = append(g.adcPinctrls, fmt.Sprintf("&adc0_chan%d_%s", adcChan, gpName))
		} else {
			g.adcPinctrls = append(g.adcPinctrls, fmt.Sprintf("&adc0_ch%d_%s_default", adcChan, gpName))
		}
	}
	return true, nil
}

func (g *dtsiGenerator) tryProcessPWM(cfg PinConfig, port string, pinNum int, gpName string) (bool, error) {
	isPWM := cfg.IO == "PWM" || cfg.PinTypeOverride == "PWM" || cfg.PinTypeOverride == "PWM_INVERT"
	if !isPWM {
		return false, nil
	}

	pwmChan, err := g.chip.ResolvePWM(cfg.PinNumber)
	if err != nil {
		return false, fmt.Errorf("failed to resolve PWM for pin %s: %v", cfg.PinNumber, err)
	}
	if gpName != "" {
		pinctrl := fmt.Sprintf("&pwm%d_gp%s%d", pwmChan, port, pinNum)
		if !g.chip.IsNPCX() {
			pinctrl = fmt.Sprintf("&pwm%d_%s_default", pwmChan, gpName)
		}
		g.pwmEnables[pwmChan] = pinctrl
	}
	return true, nil
}

func (g *dtsiGenerator) tryProcessI2C(cfg PinConfig, netName string, port string, pinNum int, gpName string) (bool, error) {
	isI2C := cfg.IO == "I2C" || cfg.PinTypeOverride == "I2C_CLOCK" || cfg.PinTypeOverride == "I2C_DATA" || cfg.PinTypeOverride == "I2C"
	if !isI2C {
		return false, nil
	}

	i2cChan, isScl, err := g.chip.ResolveI2C(cfg.PinNumber)
	if err != nil {
		return false, fmt.Errorf("failed to resolve I2C for pin %s: %v", cfg.PinNumber, err)
	}

	if !isScl {
		return true, nil
	}

	baseName := strings.ToLower(netName)
	baseName = strings.TrimSuffix(baseName, "_scl")

	label := fmt.Sprintf("i2c_%s", baseName)
	portName := baseName

	var enums []string
	if cfg.Enum != "" {
		enums = append(enums, cfg.Enum)
	}

	portNode := fmt.Sprintf("i2c%s", i2cChan)
	if g.chip.IsNPCX() {
		pinDef := g.chip.PinMap[cfg.PinNumber]
		idx := extractNPCXI2CIndex(pinDef)
		portNode = fmt.Sprintf("i2c%s_%d", i2cChan, idx)
	}

	g.i2cPorts = append(g.i2cPorts, i2cPort{
		label:    label,
		pin:      portName,
		channel:  i2cChan,
		portNode: portNode,
		enums:    enums,
	})

	if g.chip.IsNPCX() {
		sdaGP, err := findNPCXSDAGPIO(g.chip, cfg.PinNumber)
		if err != nil {
			return false, fmt.Errorf("failed to find NPCX SDA GPIO for pin %s: %v", cfg.PinNumber, err)
		}
		pinDef := g.chip.PinMap[cfg.PinNumber]
		idx := extractNPCXI2CIndex(pinDef)
		sclGP := fmt.Sprintf("gp%s%d", port, pinNum)
		gpios := []string{sdaGP, sclGP}
		slices.Sort(gpios)
		first := gpios[0]
		second := strings.TrimPrefix(gpios[1], "gp")
		pinctrl := fmt.Sprintf("&i2c%s_%d_sda_scl_%s_%s", i2cChan, idx, first, second)
		nodeName := fmt.Sprintf("i2c%s_%d", i2cChan, idx)
		g.i2cEnables[nodeName] = []string{pinctrl}
		g.i2cCtrls["i2c_ctrl"+i2cChan] = true
	} else {
		// ITE
		sdaGP, err := findITESDAGPIO(g.chip, cfg.PinNumber)
		if err != nil {
			return false, fmt.Errorf("failed to find ITE SDA GPIO for pin %s: %v", cfg.PinNumber, err)
		}
		i2cNodeTrim := "i2c" + strings.TrimSuffix(i2cChan, "ALT")
		clkPinctrl := fmt.Sprintf("&%s_clk_%s_default", i2cNodeTrim, gpName)
		sdaPinctrl := fmt.Sprintf("&%s_data_%s_default", i2cNodeTrim, sdaGP)
		nodeName := "i2c" + i2cChan
		g.i2cEnables[nodeName] = append(g.i2cEnables[nodeName], clkPinctrl, sdaPinctrl)
	}
	return true, nil
}

func (g *dtsiGenerator) tryProcessUART(cfg PinConfig, netName, gpName string) (bool, error) {
	if cfg.PinTypeOverride == "UART_RX" {
		return true, nil
	}
	if cfg.PinTypeOverride != "UART_TX" {
		return false, nil
	}
	if g.chip.IsNPCX() {
		def, ok := g.chip.PinMap[cfg.PinNumber]
		if !ok {
			return false, fmt.Errorf("failed to resolve UART for pin %s: pin %s not found in chip %s", cfg.PinNumber, cfg.PinNumber, g.chip.Name)
		}
		if !hasFunction(def, "CR_SOUT1") {
			fmt.Fprintf(os.Stderr, "Warning: skipping %s: NPCX only supports default uart1 (CR_SOUT1), pin %s is %s\n", netName, cfg.PinNumber, def)
		}
		return true, nil
	}
	soutIdx, err := g.chip.ResolveITEUART(cfg.PinNumber)
	if err != nil {
		return false, fmt.Errorf("failed to resolve UART for pin %s: %v", cfg.PinNumber, err)
	}
	uartNum := soutIdx + 1
	if uartNum == 1 {
		g.iteUarts[1] = iteUartNode{}
		return true, nil
	}
	rxGP, err := findFuncGPIO(g.chip, fmt.Sprintf("SIN%d", soutIdx))
	if err != nil {
		return false, fmt.Errorf("failed to find ITE UART RX GPIO for pin %s: %v", cfg.PinNumber, err)
	}
	g.iteUarts[uartNum] = iteUartNode{rxGP: rxGP, txGP: gpName}
	return true, nil
}

func (g *dtsiGenerator) processGPIO(cfg PinConfig, netName, port string, pinNum int, gpName string) {
	if gpName != "" {
		label := fmt.Sprintf("gpio_%s", strings.ToLower(netName))
		flags := determineGPIOFlags(cfg)
		g.gpios = append(g.gpios, gpioPin{
			label:  label,
			pin:    strings.ToLower(netName),
			port:   port,
			pinNum: pinNum,
			flags:  flags,
			enum:   cfg.Enum,
		})
	}
}

func (g *dtsiGenerator) processUnusedPin(cfg PinConfig, netName, port string, pinNum int) {
	g.unusedPins = append(g.unusedPins, unusedPin{
		comment: netName,
		port:    port,
		pinNum:  pinNum,
		flags:   determineUnusedFlags(cfg),
	})
}

func (g *dtsiGenerator) write() error {
	ew := &errWriter{w: g.w}
	g.w = ew

	// Sort lists deterministically
	slices.SortStableFunc(g.gpios, func(a, b gpioPin) int { return cmp.Compare(a.pin, b.pin) })
	slices.SortStableFunc(g.adcs, func(a, b adcPin) int { return cmp.Compare(a.pin, b.pin) })
	slices.SortStableFunc(g.i2cPorts, func(a, b i2cPort) int { return cmp.Compare(a.pin, b.pin) })
	slices.SortStableFunc(g.unusedPins, func(a, b unusedPin) int {
		return cmp.Or(cmp.Compare(a.port, b.port), cmp.Compare(a.pinNum, b.pinNum))
	})
	slices.Sort(g.adcPinctrls)

	// Write header
	fmt.Fprintln(g.w, "/* Copyright 2026 The ChromiumOS Authors")
	fmt.Fprintln(g.w, " * Use of this source code is governed by a BSD-style license that can be")
	fmt.Fprintln(g.w, " * found in the LICENSE file.")
	fmt.Fprintln(g.w, " *")
	fmt.Fprintln(g.w, " * This file is auto-generated - do not edit!")
	fmt.Fprintln(g.w, " */")
	fmt.Fprintln(g.w)
	fmt.Fprintln(g.w, "/ {")
	fmt.Fprintln(g.w)

	// Write ADC channels
	if len(g.adcs) > 0 {
		fmt.Fprintln(g.w, "\tnamed-adc-channels {")
		fmt.Fprintln(g.w, "\t\tcompatible = \"named-adc-channels\";")
		fmt.Fprintln(g.w)
		for _, adc := range g.adcs {
			fmt.Fprintf(g.w, "\t\t%s: %s {\n", adc.label, adc.pin)
			if adc.enum != "" {
				fmt.Fprintf(g.w, "\t\t\tenum-name = \"%s\";\n", adc.enum)
			}
			fmt.Fprintf(g.w, "\t\t\tio-channels = <&adc0 %d>;\n", adc.channel)
			fmt.Fprintln(g.w, "\t\t};")
		}
		fmt.Fprintln(g.w, "\t};")
		fmt.Fprintln(g.w)
	}

	// Write Named GPIOs
	if len(g.gpios) > 0 {
		fmt.Fprintln(g.w, "\tnamed-gpios {")
		fmt.Fprintln(g.w, "\t\tcompatible = \"named-gpios\";")
		fmt.Fprintln(g.w)
		for _, gp := range g.gpios {
			fmt.Fprintf(g.w, "\t\t%s: %s {\n", gp.label, gp.pin)
			fmt.Fprintf(g.w, "\t\t\tgpios = <&gpio%s %d %s>;\n", gp.port, gp.pinNum, gp.flags)
			if gp.enum != "" {
				fmt.Fprintf(g.w, "\t\t\tenum-name = \"%s\";\n", gp.enum)
			}
			fmt.Fprintln(g.w, "\t\t};")
		}
		fmt.Fprintln(g.w, "\t};")
		fmt.Fprintln(g.w)
	}

	// Write Named I2C ports
	if len(g.i2cPorts) > 0 {
		fmt.Fprintln(g.w, "\tnamed-i2c-ports {")
		fmt.Fprintln(g.w, "\t\tcompatible = \"named-i2c-ports\";")
		fmt.Fprintln(g.w)
		for _, i2c := range g.i2cPorts {
			fmt.Fprintf(g.w, "\t\t%s: %s {\n", i2c.label, i2c.pin)
			fmt.Fprintf(g.w, "\t\t\ti2c-port = <&%s>;\n", strings.TrimSuffix(i2c.portNode, "ALT"))
			if len(i2c.enums) > 0 {
				fmt.Fprintf(g.w, "\t\t\tenum-names = \"%s\";\n", strings.Join(i2c.enums, "\", \""))
			}
			fmt.Fprintln(g.w, "\t\t};")
		}
		fmt.Fprintln(g.w, "\t};")
		fmt.Fprintln(g.w)
	}

	// Write Unused pins
	if len(g.unusedPins) > 0 {
		fmt.Fprintln(g.w, "\tunused-pins {")
		fmt.Fprintln(g.w, "\t\tcompatible = \"unused-gpios\";")
		fmt.Fprintln(g.w, "\t\tunused-gpios =")
		for i, up := range g.unusedPins {
			if up.comment != "" {
				fmt.Fprintf(g.w, "\t\t\t/* %s */\n", up.comment)
			}
			sep := ","
			if i == len(g.unusedPins)-1 {
				sep = ";"
			}
			fmt.Fprintf(g.w, "\t\t\t<&gpio%s %d %s>%s\n", up.port, up.pinNum, up.flags, sep)
		}
		fmt.Fprintln(g.w, "\t};")
		fmt.Fprintln(g.w)
	}

	fmt.Fprintln(g.w, "};")
	fmt.Fprintln(g.w)

	// Write ADC pinctrl
	if len(g.adcPinctrls) > 0 {
		fmt.Fprintln(g.w, "&adc0 {")
		fmt.Fprintln(g.w, "\tstatus = \"okay\";")
		if len(g.adcPinctrls) == 1 {
			fmt.Fprintf(g.w, "\tpinctrl-0 = <%s>;\n", g.adcPinctrls[0])
		} else {
			fmt.Fprintln(g.w, "\tpinctrl-0 = <")
			for _, p := range g.adcPinctrls {
				fmt.Fprintf(g.w, "\t\t%s\n", p)
			}
			fmt.Fprintln(g.w, "\t>;")
		}
		fmt.Fprintln(g.w, "\tpinctrl-names = \"default\";")
		fmt.Fprintln(g.w, "};")
		fmt.Fprintln(g.w)
	}

	// Write I2C pinctrl
	i2cNodes := slices.Collect(maps.Keys(g.i2cEnables))
	slices.Sort(i2cNodes)

	for _, nodeName := range i2cNodes {
		pinctrls := g.i2cEnables[nodeName]
		fmt.Fprintf(g.w, "&%s {\n", nodeName)
		fmt.Fprintln(g.w, "\tstatus = \"okay\";")
		if len(pinctrls) == 1 {
			fmt.Fprintf(g.w, "\tpinctrl-0 = <%s>;\n", pinctrls[0])
		} else {
			fmt.Fprintln(g.w, "\tpinctrl-0 = <")
			for _, p := range pinctrls {
				fmt.Fprintf(g.w, "\t\t%s\n", p)
			}
			fmt.Fprintln(g.w, "\t>;")
		}
		fmt.Fprintln(g.w, "\tpinctrl-names = \"default\";")
		fmt.Fprintln(g.w, "};")
		fmt.Fprintln(g.w)
	}

	i2cCtrls := slices.Collect(maps.Keys(g.i2cCtrls))
	slices.Sort(i2cCtrls)

	for _, ctrlNode := range i2cCtrls {
		fmt.Fprintf(g.w, "&%s {\n", ctrlNode)
		fmt.Fprintln(g.w, "\tstatus = \"okay\";")
		fmt.Fprintln(g.w, "};")
		fmt.Fprintln(g.w)
	}

	// Write PWM pinctrl
	pwmChans := slices.Collect(maps.Keys(g.pwmEnables))
	slices.Sort(pwmChans)

	for _, ch := range pwmChans {
		fmt.Fprintf(g.w, "&pwm%d {\n", ch)
		fmt.Fprintln(g.w, "\tstatus = \"okay\";")
		fmt.Fprintf(g.w, "\tpinctrl-0 = <%s>;\n", g.pwmEnables[ch])
		fmt.Fprintln(g.w, "\tpinctrl-names = \"default\";")
		fmt.Fprintln(g.w, "};")
		fmt.Fprintln(g.w)
	}

	// Write Tach pinctrl
	tachChans := slices.Collect(maps.Keys(g.tachEnables))
	slices.Sort(tachChans)

	for _, ch := range tachChans {
		portLetter := strings.ToUpper(g.tachPorts[ch])
		fmt.Fprintf(g.w, "&tach%d {\n", ch)
		fmt.Fprintln(g.w, "\tstatus = \"okay\";")
		fmt.Fprintf(g.w, "\tpinctrl-0 = <%s>;\n", g.tachEnables[ch])
		fmt.Fprintln(g.w, "\tpinctrl-names = \"default\";")
		if g.chip.IsNPCX() {
			fmt.Fprintf(g.w, "\tport = <NPCX_TACH_PORT_%s>;\n", portLetter)
			fmt.Fprintln(g.w, "\tsample-clk = <NPCX_TACH_FREQ_LFCLK>;")
			fmt.Fprintln(g.w, "\tpulses-per-round = <2>;")
		} else {
			fmt.Fprintf(g.w, "\tchannel = <IT8XXX2_TACH_CHANNEL_%s>;\n", portLetter)
			fmt.Fprintln(g.w, "\tpulses-per-round = <2>;")
		}
		fmt.Fprintln(g.w, "};")
		fmt.Fprintln(g.w)
	}

	// Write UART pinctrl
	if _, hasUart1 := g.iteUarts[1]; len(g.iteUarts) > 0 && !hasUart1 {
		fmt.Fprintln(g.w, "&uart1 {")
		fmt.Fprintln(g.w, "\tstatus = \"disabled\";")
		fmt.Fprintln(g.w, "};")
		fmt.Fprintln(g.w)
		fmt.Fprintln(g.w, "&ite_uart1_wrapper {")
		fmt.Fprintln(g.w, "\tstatus = \"disabled\";")
		fmt.Fprintln(g.w, "};")
		fmt.Fprintln(g.w)

		uartNums := slices.Collect(maps.Keys(g.iteUarts))
		slices.Sort(uartNums)
		for _, num := range uartNums {
			u := g.iteUarts[num]
			fmt.Fprintf(g.w, "&uart%d {\n", num)
			fmt.Fprintln(g.w, "\tstatus = \"okay\";")
			fmt.Fprintln(g.w, "\tcurrent-speed = <115200>;")
			fmt.Fprintln(g.w, "};")
			fmt.Fprintln(g.w)
			fmt.Fprintf(g.w, "&ite_uart%d_wrapper {\n", num)
			fmt.Fprintln(g.w, "\tstatus = \"okay\";")
			fmt.Fprintln(g.w, "\tpinctrl-0 = <")
			fmt.Fprintf(g.w, "\t\t&uart%d_rx_%s_default\n", num, u.rxGP)
			fmt.Fprintf(g.w, "\t\t&uart%d_tx_%s_default\n", num, u.txGP)
			fmt.Fprintln(g.w, "\t>;")
			fmt.Fprintln(g.w, "\tpinctrl-names = \"default\";")
			fmt.Fprintln(g.w, "};")
			fmt.Fprintln(g.w)
		}
	}

	// Write Keyboard matrix pinctrl
	if len(g.kbPins) > 0 {
		slices.Sort(g.kbPins)
		slices.Sort(g.kbSleepPins)
		fmt.Fprintln(g.w, "&kbd {")
		fmt.Fprintln(g.w, "\tstatus = \"okay\";")
		fmt.Fprintln(g.w, "\tpinctrl-0 = <")
		for _, p := range g.kbPins {
			fmt.Fprintf(g.w, "\t\t%s\n", p)
		}
		fmt.Fprintln(g.w, "\t>;")
		fmt.Fprintln(g.w, "\tpinctrl-1 = <")
		for _, p := range g.kbSleepPins {
			fmt.Fprintf(g.w, "\t\t%s\n", p)
		}
		fmt.Fprintln(g.w, "\t>;")
		fmt.Fprintln(g.w, "\tpinctrl-names = \"default\", \"sleep\";")
		rowSize := 0
		if g.maxKsi >= 0 {
			rowSize = g.maxKsi + 1
		}
		colSize := 0
		if g.maxKso >= 0 {
			colSize = g.maxKso + 1
		}
		fmt.Fprintf(g.w, "\trow-size = <%d>;\n", rowSize)
		fmt.Fprintf(g.w, "\tcol-size = <%d>;\n", colSize)
		fmt.Fprintln(g.w, "};")
	}

	return ew.err
}

func findFuncGPIO(chip *Chip, fn string) (string, error) {
	var pins []string
	if len(chip.PkgPins) > 0 {
		pins = slices.Clone(chip.PkgPins)
	} else {
		pins = slices.Collect(maps.Keys(chip.PinMap))
		slices.Sort(pins)
	}

	for _, pinNum := range pins {
		def := chip.PinMap[pinNum]
		if hasFunction(def, fn) {
			port, pin, err := chip.ResolveGPIO(pinNum)
			if err != nil {
				return "", err
			}
			return fmt.Sprintf("gp%s%d", port, pin), nil
		}
	}
	return "", fmt.Errorf("matching pin function %s not found in chip map", fn)
}

func findNPCXSDAGPIO(chip *Chip, sclPinNum string) (string, error) {
	sclDef := chip.PinMap[sclPinNum]
	m := npcxSclPinRegex.FindStringSubmatch(sclDef)
	if m == nil {
		return "", fmt.Errorf("not an NPCX SCL pin definition: %s", sclDef)
	}
	sdaFunc := fmt.Sprintf("I2C%s_SDA%s", m[1], m[2])
	return findFuncGPIO(chip, sdaFunc)
}

func findITESDAGPIO(chip *Chip, sclPinNum string) (string, error) {
	sclDef := chip.PinMap[sclPinNum]
	m := iteScl.FindStringSubmatch(sclDef)
	if m == nil {
		return "", fmt.Errorf("not an ITE SCL pin definition: %s", sclDef)
	}
	sdaFunc := fmt.Sprintf("SMDAT%s", m[1])
	return findFuncGPIO(chip, sdaFunc)
}

func extractNPCXI2CIndex(pinDef string) int {
	m := npcxI2cIndexRegex.FindStringSubmatch(pinDef)
	if m != nil {
		idx, err := strconv.Atoi(m[2])
		if err == nil {
			return idx
		}
	}
	return 0
}

func cleanNetName(name string) string {
	return netNameReplacer.Replace(name)
}

func isGPIO(cfg PinConfig) bool {
	if cfg.IO == "UNUSED" || cfg.PinTypeOverride == "UNUSED" {
		return false
	}
	if cfg.Dir == "IN" || cfg.Dir == "OUT" {
		return true
	}
	switch cfg.PinTypeOverride {
	case "INPUT", "INPUT_L", "OUTPUT", "OUTPUT_L", "OUTPUT_ACTIVE_L", "OUTPUT_ACTIVE_H", "ODR_ACTIVE_L", "ODR_ACTIVE_H", "OUTPUT_ODL", "OUTPUT_ODR":
		return true
	}
	return false
}

func determineUnusedFlags(cfg PinConfig) string {
	var flags []string
	isOutput := cfg.PinTypeOverride == "UNUSED_OUT"
	if isOutput {
		flags = append(flags, "GPIO_OUTPUT", "GPIO_ACTIVE_HIGH")
	} else {
		flags = append(flags, "GPIO_INPUT")
	}
	if cfg.Pull == "UP" {
		flags = append(flags, "GPIO_PULL_UP")
	} else if cfg.Pull == "DOWN" {
		flags = append(flags, "GPIO_PULL_DOWN")
	}
	if isOutput {
		if cfg.InitialCondition == "HIGH" {
			flags = append(flags, "GPIO_OUTPUT_INIT_HIGH")
		} else if cfg.InitialCondition == "LOW" {
			flags = append(flags, "GPIO_OUTPUT_INIT_LOW")
		}
	}
	return "(" + strings.Join(flags, " | ") + ")"
}

func hasFunction(pinDef, function string) bool {
	parts := strings.Split(pinDef, "/")
	for _, part := range parts {
		if strings.TrimSpace(part) == function {
			return true
		}
	}
	return false
}

func findDigits(s string) string {
	return digitsRegex.FindString(s)
}

func determineGPIOFlags(cfg PinConfig) string {
	var flags []string

	isOutput := cfg.Dir == "OUT" ||
		cfg.PinTypeOverride == "OUTPUT" ||
		cfg.PinTypeOverride == "OUTPUT_L" ||
		cfg.PinTypeOverride == "OUTPUT_H" ||
		cfg.PinTypeOverride == "OUTPUT_ACTIVE_L" ||
		cfg.PinTypeOverride == "OUTPUT_ACTIVE_H" ||
		cfg.PinTypeOverride == "OUTPUT_ODL" ||
		cfg.PinTypeOverride == "OUTPUT_ODR" ||
		cfg.PinTypeOverride == "ODR_ACTIVE_L" ||
		cfg.PinTypeOverride == "ODR_ACTIVE_H"

	if isOutput {
		flags = append(flags, "GPIO_OUTPUT")
	} else {
		flags = append(flags, "GPIO_INPUT")
	}

	isOD := cfg.IO == "OD" ||
		cfg.PinTypeOverride == "OUTPUT_ODL" ||
		cfg.PinTypeOverride == "OUTPUT_ODR" ||
		cfg.PinTypeOverride == "ODR_ACTIVE_L" ||
		cfg.PinTypeOverride == "ODR_ACTIVE_H"

	if isOD && (isOutput || cfg.Dir == "IO") {
		flags = append(flags, "GPIO_OPEN_DRAIN")
	}

	isActiveLow := cfg.PinTypeOverride == "INPUT_L" ||
		cfg.PinTypeOverride == "OUTPUT_L" ||
		cfg.PinTypeOverride == "OUTPUT_ACTIVE_L" ||
		cfg.PinTypeOverride == "OUTPUT_ODL" ||
		cfg.PinTypeOverride == "ODR_ACTIVE_L"

	if !isActiveLow && (cfg.PinTypeOverride == "" || cfg.PinTypeOverride == "INPUT" || cfg.PinTypeOverride == "OUTPUT") {
		cleanName := strings.ToLower(cfg.NetName)
		if strings.HasSuffix(cleanName, "_l") || strings.HasSuffix(cleanName, "_odl") || strings.HasSuffix(cleanName, "_low") {
			isActiveLow = true
		}
	}

	if isActiveLow {
		flags = append(flags, "GPIO_ACTIVE_LOW")
	} else {
		flags = append(flags, "GPIO_ACTIVE_HIGH")
	}

	if cfg.Pull == "UP" {
		flags = append(flags, "GPIO_PULL_UP")
	} else if cfg.Pull == "DOWN" {
		flags = append(flags, "GPIO_PULL_DOWN")
	}

	if isOutput {
		if cfg.InitialCondition == "HIGH" {
			flags = append(flags, "GPIO_OUTPUT_INIT_HIGH")
		} else if cfg.InitialCondition == "LOW" {
			flags = append(flags, "GPIO_OUTPUT_INIT_LOW")
		}
	}

	if cfg.Voltage == "1.8V" {
		flags = append(flags, "GPIO_VOLTAGE_1P8")
	}

	return "(" + strings.Join(flags, " | ") + ")"
}

type errWriter struct {
	w   io.Writer
	err error
}

func (ew *errWriter) Write(p []byte) (int, error) {
	if ew.err != nil {
		return 0, ew.err
	}
	var n int
	n, ew.err = ew.w.Write(p)
	return n, ew.err
}
