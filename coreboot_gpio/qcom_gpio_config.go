// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package coreboot_gpio

import (
	"fmt"
	"io"
	"os"
	"regexp"
	"sort"
	"strconv"
	"strings"
)

// QcomPinRow represents the essential fields parsed from a row of the Qualcomm SoC GPIO map CSV.
type QcomPinRow struct {
	Pin          int
	DefaultPull  string // "PD" or "PU"
	RawNetName   string
	EffectiveNet string
	FunctionDesc string
	AltFuncs     [2]string // Alternate function 1 and 2 (used for QUP SE and special function detection)
	EGPIOEnable  string
}

// QcomMacroDef represents a symbolic #define in board.h.
type QcomMacroDef struct {
	Name    string
	Value   string
	Comment string
}

// QcomEarlyGPIOState represents the initial state in early_gpio.c.
type QcomEarlyGPIOState string

const (
	EarlyGPIOInput      QcomEarlyGPIOState = "GPIO_CFG_IN"
	EarlyGPIOOutputLow  QcomEarlyGPIOState = "GPIO_CFG_OUT_LOW"
	EarlyGPIOOutputHigh QcomEarlyGPIOState = "GPIO_CFG_OUT_HIGH"
)

// QcomEarlyGPIOEntry represents an entry in default_gpio_table[] in early_gpio.c.
type QcomEarlyGPIOEntry struct {
	MacroName string
	Pin       int
	NetName   string
	State     QcomEarlyGPIOState
}

// QcomQUPProtocol represents the QUPv3 Serial Engine protocol.
type QcomQUPProtocol string

const (
	QUPProtoI2C  QcomQUPProtocol = "SE_PROTOCOL_I2C"
	QUPProtoSPI  QcomQUPProtocol = "SE_PROTOCOL_SPI"
	QUPProtoUART QcomQUPProtocol = "SE_PROTOCOL_UART"
)

// QcomQUPRole categorizes how a QUP Serial Engine is initialized in coreboot.
type QcomQUPRole int

const (
	QUPRoleBootblockTPM QcomQUPRole = iota
	QUPRoleBootblockECSPI
	QUPRoleRamstageEarlyADSP
	QUPRoleRamstageDebugUART
	QUPRoleRamstageTouchI2C
	QUPRoleRamstageBTUART
	QUPRoleRamstageTrackpadI2C
	QUPRoleRamstageUSBC0RetimerI2C
	QUPRoleRamstageUSBC1RetimerI2C
	QUPRoleRamstageUSBARetimerI2C
	QUPRoleRamstageAppsI2C
	QUPRoleRamstageFPSPI
	QUPRoleOther
)

// QcomQUPBus represents a grouped QUPv3 Serial Engine bus.
type QcomQUPBus struct {
	EnumName string // e.g., "QUPV3_1_SE5"
	Protocol QcomQUPProtocol
	Mode     string // "MIXED", "FIFO", "GSI"
	Role     QcomQUPRole
	Comment  string
}

// QcomBoardConfig holds all categorized GPIO and QUP configurations across stages.
type QcomBoardConfig struct {
	Macros         []QcomMacroDef
	EarlyInputs    []QcomEarlyGPIOEntry
	EarlyOutputs   []QcomEarlyGPIOEntry
	QUPBuses       []QcomQUPBus
	ECIntMacro     string
	GSCIntMacro    string
	FPRstMacro     string
	FPEnRailsMacro string
	FPBoot0Macro   string
	FPIntMacro     string

	// Display / eDP
	PanelPowerMacro string
	PanelHPDMacro   string

	// Lid & SD card
	LidOpenMacro string
	SDCardMacro  string

	// USB Type-C & Host
	USBC1En3P3Macro    string
	USBC1En1P8Macro    string
	USBC1En0P9Macro    string
	USBC1RetimerRst    string
	USBCamRstMacro     string
	USBCamEnMacro      string
	EUSBRepeaterResets []string

	// Audio SoundWire / I2S & EGPIO
	SndwSCLMacro    string
	SndwSCLFunc     int
	SndwSCLFuncNote string
	SndwSDAMacro    string
	SndwSDAFunc     int
	SndwSDAFuncNote string
	SndwAmp0EnMacro string
	SndwAmp1EnMacro string
	I2SPins         []string

	// Strapping IDs (boardid.c)
	BoardIDMacros []string
	RamIDMacros   []string
}

var (
	gpioPinRegex = regexp.MustCompile(`^GPIO_(\d+)$`)
	qupFuncRegex = regexp.MustCompile(`^QUP(\d+)_SE(\d+)_L(\d+)$`)
	nonIdentChar = regexp.MustCompile(`[^A-Z0-9_]+`)
)

// processQcomGPIOs parses the Qcom GPIO CSV records, classifies pins across stages,
// and writes the consolidated C configuration to outputWriter.
func processQcomGPIOs(records [][]string, outputWriter io.Writer) error {
	rows, err := parseQcomCSV(records)
	if err != nil {
		return fmt.Errorf("error parsing Qcom CSV: %w", err)
	}

	cfg := classifyQcomPins(rows)

	fmt.Fprintln(os.Stderr, "Successfully parsed Qcom GPIO pin data. Generating C source...")
	generateQcomCode(cfg, outputWriter)
	return nil
}

func normalizeHeaderCell(s string) string {
	s = strings.ReplaceAll(s, "\r\n", " ")
	s = strings.ReplaceAll(s, "\n", " ")
	s = strings.ReplaceAll(s, "\r", " ")
	return strings.Join(strings.Fields(s), " ")
}

func isDisconnectNet(s string) bool {
	s = strings.TrimSpace(s)
	if s == "" || s == "-" || s == "–" || s == "—" {
		return true
	}
	u := strings.ToUpper(s)
	if u == "NC" || strings.HasPrefix(u, "NC ") || strings.HasPrefix(u, "NC_") ||
		strings.HasPrefix(u, "TP ") || strings.HasPrefix(u, "TP(") || strings.HasSuffix(u, "_TP") ||
		strings.Contains(u, "NOT CONNECTED") || strings.Contains(u, "UNUSED") || strings.Contains(u, "(TBD)") {
		return true
	}
	return false
}

// parseQcomCSV parses the Qcom SoC GPIO map CSV rows and auto-detects the board
// net name and function columns by excluding reference board columns (CRD / RVP).
func parseQcomCSV(records [][]string) ([]QcomPinRow, error) {
	if len(records) < 2 {
		return nil, fmt.Errorf("CSV data must have a header row and at least one data row")
	}

	headerRowIdx := -1
	var headers []string
	searchLimit := 5
	if len(records) < searchLimit {
		searchLimit = len(records)
	}

	for r := 0; r < searchLimit; r++ {
		norm := make([]string, len(records[r]))
		hasPadName := false
		hasPull := false
		for c, cell := range records[r] {
			norm[c] = normalizeHeaderCell(cell)
			lower := strings.ToLower(norm[c])
			if lower == "pad name" {
				hasPadName = true
			}
			if strings.HasPrefix(lower, "pull") {
				hasPull = true
			}
		}
		if hasPadName && hasPull {
			headerRowIdx = r
			headers = norm
			break
		}
	}

	if headerRowIdx == -1 {
		return nil, fmt.Errorf("expected Qcom header row containing 'Pad name' and 'Pull' not found")
	}

	padNameCol := -1
	pullCol := -1
	refFuncCol := -1
	rawNetCol := -1
	schemNetCol := -1
	funcCol := -1
	altFuncCols := [2]int{-1, -1}
	egpioEnableCol := -1

	isRefHeader := func(lower string) bool {
		return strings.Contains(lower, "crd") || strings.Contains(lower, "rvp") ||
			strings.Contains(lower, "reference")
	}

	for c, h := range headers {
		lower := strings.ToLower(h)
		switch {
		case lower == "pad name":
			padNameCol = c
		case strings.HasPrefix(lower, "pull"):
			pullCol = c
		case strings.Contains(lower, "egpio enable"):
			egpioEnableCol = c
		case strings.HasPrefix(lower, "alternate function 1"):
			altFuncCols[0] = c
		case strings.HasPrefix(lower, "alternate function 2"):
			altFuncCols[1] = c
		case isRefHeader(lower) && strings.Contains(lower, "function"):
			refFuncCol = c
		default:
			if isRefHeader(lower) || strings.HasPrefix(lower, "function 0") ||
				strings.Contains(lower, "alternate function") ||
				strings.Contains(lower, "enterprise management") {
				continue
			}
			if strings.Contains(lower, "schematic net name") {
				schemNetCol = c
			} else if strings.Contains(lower, "net name") {
				rawNetCol = c
			} else if strings.Contains(lower, "function") {
				funcCol = c
			}
		}
	}

	if padNameCol == -1 || pullCol == -1 {
		return nil, fmt.Errorf("missing required 'Pad name' or 'Pull' columns")
	}
	if schemNetCol == -1 && rawNetCol == -1 {
		return nil, fmt.Errorf("no board net name column found in CSV header")
	}

	getCell := func(row []string, col int) string {
		if col < 0 || col >= len(row) {
			return ""
		}
		return strings.TrimSpace(row[col])
	}

	var rows []QcomPinRow
	for _, record := range records[headerRowIdx+1:] {
		padName := getCell(record, padNameCol)
		if padName == "" {
			continue
		}
		m := gpioPinRegex.FindStringSubmatch(padName)
		if len(m) < 2 {
			continue
		}
		pinNum, err := strconv.Atoi(m[1])
		if err != nil {
			continue
		}

		pullRaw := getCell(record, pullCol)
		defaultPull := "PD"
		if strings.HasPrefix(strings.ToUpper(pullRaw), "PU") {
			defaultPull = "PU"
		}

		rawNet := getCell(record, rawNetCol)
		schemNet := getCell(record, schemNetCol)
		boardFunc := normalizeHeaderCell(getCell(record, funcCol))
		funcDesc := boardFunc
		if funcDesc == "" && refFuncCol >= 0 {
			refFunc := normalizeHeaderCell(getCell(record, refFuncCol))
			if !isDisconnectNet(refFunc) {
				funcDesc = refFunc
			}
		}

		// Determine effective net name using the board's own function column.
		effectiveNet := resolveEffectiveNet(rawNet, schemNet, boardFunc, rawNetCol >= 0, schemNetCol >= 0)

		row := QcomPinRow{
			Pin:          pinNum,
			DefaultPull:  defaultPull,
			RawNetName:   rawNet,
			EffectiveNet: effectiveNet,
			FunctionDesc: funcDesc,
			AltFuncs: [2]string{
				getCell(record, altFuncCols[0]),
				getCell(record, altFuncCols[1]),
			},
			EGPIOEnable: getCell(record, egpioEnableCol),
		}
		rows = append(rows, row)
	}

	return rows, nil
}

func resolveEffectiveNet(rawNet, schemNet, funcDesc string, hasRawCol, hasSchemCol bool) string {
	lowerFunc := strings.ToLower(funcDesc)
	if lowerFunc == "not connected" || lowerFunc == "unused" {
		return ""
	}
	if hasRawCol && rawNet != "" && isDisconnectNet(rawNet) {
		return ""
	}
	if hasSchemCol && schemNet != "" && isDisconnectNet(schemNet) {
		return ""
	}
	if hasSchemCol {
		if !isDisconnectNet(schemNet) {
			return schemNet
		}
		// Fallback for active SAR sensor interrupt nets when schematic column cell is blank.
		upperRaw := strings.ToUpper(rawNet)
		if !isDisconnectNet(rawNet) && strings.HasPrefix(upperRaw, "SAR") && strings.Contains(upperRaw, "INT") {
			return rawNet
		}
		return ""
	}
	if !isDisconnectNet(rawNet) {
		return rawNet
	}
	return ""
}

func shouldSkipQcomPin(r QcomPinRow) bool {
	if r.EffectiveNet == "" {
		return true
	}
	uNet := strings.ToUpper(r.EffectiveNet)
	uRaw := strings.ToUpper(r.RawNetName)
	uEGPIO := strings.ToUpper(r.EGPIOEnable)

	// Skip hardware POR boot straps, SoC QSPI flash (handled by soc/qualcomm/common/qspi.c),
	// and SoC reset/throttle special function pins.
	if strings.Contains(uNet, "BOOT_CONFIG") || strings.Contains(uNet, "FORCED_USB_BOOT") ||
		strings.Contains(uNet, "RESOUT") || strings.Contains(uNet, "SYS_THROTTLE") ||
		strings.Contains(uNet, "SOC_SPI_0_") || strings.HasPrefix(uNet, "QSPI") ||
		strings.HasPrefix(strings.ToUpper(r.AltFuncs[0]), "QSPI") {
		return true
	}

	// Skip SSC Sensor Island pins (owned by ADSP/SSC firmware).
	if strings.HasPrefix(uEGPIO, "SSC_") {
		return true
	}

	// For LPASS island pins, keep speaker Audio pins (WSA SoundWire clock/data, WSA amp enables,
	// or I2S speaker lines) used for depthcharge dev-mode beep, and Camera PMIC enable;
	// skip internal DMIC microphones, WCD headphone jack, and WCN Slimbus lines.
	if strings.HasPrefix(uEGPIO, "LPASS_") {
		isWSASndw := strings.Contains(uNet, "SNDW_0_SCL") || strings.Contains(uNet, "SNDW_0_SDA") ||
			strings.Contains(uRaw, "WSA_SWR")
		isWSAEn := (strings.HasPrefix(uNet, "WSA") || strings.Contains(uNet, "AMP")) && strings.Contains(uNet, "_EN")
		isSpeakerI2S := strings.Contains(uNet, "I2S") || strings.Contains(uNet, "MI2S")
		isCamPmicEn := strings.Contains(uNet, "CAM_PM_EM_EN")
		if !isWSASndw && !isWSAEn && !isSpeakerI2S && !isCamPmicEn {
			return true
		}
	}

	// Skip MIPI Camera MCLK, CCI I2C, privacy shutter/LED, and raw MIPI camera resets.
	if strings.HasPrefix(uNet, "CAM_MCLK") || strings.HasPrefix(uNet, "CAM_AON_MCLK") ||
		strings.HasPrefix(uNet, "CAM_PRIVACY") || strings.HasPrefix(uNet, "CAM0_RESET") ||
		strings.HasPrefix(uNet, "CAM4_RESET") || strings.HasPrefix(uNet, "AON_DRI_CAM") ||
		strings.Contains(uRaw, "IRCAM") || strings.HasPrefix(strings.ToUpper(r.AltFuncs[0]), "CCI_I2C") {
		return true
	}

	// Skip WCN dedicated control lines, PCIe CLKREQ mux pins, and USB4 sideband/CC_DIR/BIST pins.
	if strings.HasPrefix(uNet, "WCN_") || strings.HasPrefix(uNet, "CLKREQ_PCIE") ||
		strings.Contains(uNet, "_SBRX") || strings.Contains(uNet, "_SBTX") ||
		strings.Contains(uNet, "_CC_DIR") || strings.Contains(uNet, "_SBU_") ||
		strings.Contains(uNet, "SELF_TEST") || strings.Contains(uNet, "EURO_US_HS_SEL") ||
		strings.Contains(uNet, "SOC_HDMI_HPD") || strings.HasSuffix(uNet, "_CHG_DISABLE") {
		return true
	}

	return false
}

// matchQUPPin checks if a pin is part of a QUPv3 Serial Engine bus (I2C, SPI, or UART)
// and returns (controller, se, protocol, matched).
func matchQUPPin(r QcomPinRow) (int, int, QcomQUPProtocol, bool) {
	uNet := strings.ToUpper(r.EffectiveNet + " " + r.RawNetName)
	uFunc := strings.ToUpper(r.FunctionDesc)

	// Exclude discrete interrupt/reset/enable GPIOs that happen to share QUP mux pads.
	if strings.Contains(uNet, "INT") || strings.Contains(uNet, "IRQ") ||
		strings.Contains(uNet, "RESET") || strings.Contains(uNet, "RST") ||
		strings.Contains(uNet, "STRAP") || strings.Contains(uNet, "WP_ODL") ||
		strings.Contains(uNet, "REG_EN") || strings.Contains(uNet, "EN_PWR") ||
		strings.Contains(uNet, "LID_CLOSE") || strings.Contains(uNet, "USB_OC") ||
		strings.Contains(uNet, "SD_CARD_DET") {
		return 0, 0, "", false
	}

	isI2C := strings.Contains(uNet, "I2C") || strings.Contains(uNet, "_SDA") || strings.Contains(uNet, "_SCL") ||
		strings.Contains(uFunc, "I2C")
	isSPI := strings.Contains(uNet, "SPI") || strings.Contains(uNet, "_MISO") || strings.Contains(uNet, "_MOSI") ||
		strings.Contains(uNet, "_SCLK") || strings.Contains(uFunc, "SPI")
	isUART := strings.Contains(uNet, "UART") || strings.Contains(uNet, "CTXD") || strings.Contains(uNet, "CRXD") ||
		strings.Contains(uFunc, "UART")

	if !isI2C && !isSPI && !isUART {
		return 0, 0, "", false
	}

	for _, af := range r.AltFuncs {
		m := qupFuncRegex.FindStringSubmatch(strings.TrimSpace(af))
		if len(m) < 4 {
			continue
		}
		ctrl, _ := strconv.Atoi(m[1])
		se, _ := strconv.Atoi(m[2])
		lane, _ := strconv.Atoi(m[3])

		if isI2C {
			// I2C always uses L0 (SDA) and L1 (SCL).
			if lane == 0 || lane == 1 {
				return ctrl, se, QUPProtoI2C, true
			}
			continue
		}
		if isSPI {
			return ctrl, se, QUPProtoSPI, true
		}
		if isUART {
			return ctrl, se, QUPProtoUART, true
		}
	}

	return 0, 0, "", false
}

func classifyQUPBusRole(proto QcomQUPProtocol, combinedText string) (QcomQUPRole, string, string) {
	u := strings.ToUpper(combinedText)
	switch proto {
	case QUPProtoSPI:
		if strings.Contains(u, "EC_SPI") || strings.Contains(u, "EC TO SOC SPI") {
			return QUPRoleBootblockECSPI, "MIXED", "EC SPI"
		}
		if strings.Contains(u, "FP_SPI") || strings.Contains(u, "FPS SPI") || strings.Contains(u, "FINGERPRINT") {
			return QUPRoleRamstageFPSPI, "MIXED", "Fingerprint SPI"
		}
	case QUPProtoI2C:
		if strings.Contains(u, "GSC") || strings.Contains(u, "TPM") {
			return QUPRoleBootblockTPM, "MIXED", "H1/TPM I2C"
		}
		if strings.Contains(u, "ADSP") || strings.Contains(u, "CHARGER") || strings.Contains(u, "FUEL GAUGE") {
			return QUPRoleRamstageEarlyADSP, "MIXED", "ADSP I2C (Charger/Fuel gauge)"
		}
		if strings.Contains(u, "TOUCHSCREEN") || strings.Contains(u, "_TS_") {
			return QUPRoleRamstageTouchI2C, "MIXED", "Touch I2C"
		}
		if strings.Contains(u, "TRACKPAD") || strings.Contains(u, "_TP_") {
			return QUPRoleRamstageTrackpadI2C, "MIXED", "Trackpad I2C"
		}
		if strings.Contains(u, "USBC0") || strings.Contains(u, "USB0 CONTROLLER") {
			return QUPRoleRamstageUSBC0RetimerI2C, "MIXED", "USB-C0 Re-Timer I2C"
		}
		if strings.Contains(u, "USBC1") || strings.Contains(u, "USB1 CONTROLLER") {
			return QUPRoleRamstageUSBC1RetimerI2C, "MIXED", "USB-C1 Re-Timer I2C"
		}
		if strings.Contains(u, "USBA") || strings.Contains(u, "USB-A") || strings.Contains(u, "USB2 CONTROLLER") {
			return QUPRoleRamstageUSBARetimerI2C, "MIXED", "USB-A retimer"
		}
		if strings.Contains(u, "APPS") || strings.Contains(u, "EUSB") {
			return QUPRoleRamstageAppsI2C, "GSI", "eUSB repeater"
		}
	case QUPProtoUART:
		if strings.Contains(u, "DEBUG UART") || strings.Contains(u, "DBG_UART") || strings.Contains(u, "DRXD") {
			if strings.Contains(u, "BLUETOOTH") || strings.Contains(u, "UART_0") {
				return QUPRoleRamstageBTUART, "FIFO", "BT UART"
			}
			return QUPRoleRamstageDebugUART, "FIFO", "Debug UART"
		}
		if strings.Contains(u, "BLUETOOTH") || strings.Contains(u, "BT UART") || strings.Contains(u, "UART_0") {
			return QUPRoleRamstageBTUART, "FIFO", "BT UART"
		}
	}
	return QUPRoleOther, "MIXED", "Auxiliary QUP bus"
}

func sanitizeNetToMacro(net string) string {
	s := strings.ToUpper(strings.TrimSpace(net))
	s = strings.TrimSuffix(s, "_LS")
	if strings.HasSuffix(s, "_#") || strings.HasSuffix(s, "#") {
		s = strings.TrimSuffix(s, "_#")
		s = strings.TrimSuffix(s, "#")
		if !strings.HasSuffix(s, "_L") && !strings.HasSuffix(s, "_ODL") {
			s = s + "_L"
		}
	}
	s = nonIdentChar.ReplaceAllString(s, "_")
	s = strings.Trim(s, "_")
	if !strings.HasPrefix(s, "GPIO_") {
		s = "GPIO_" + s
	}
	return s
}

// classifyQcomPins categorizes parsed Qcom GPIO rows into stage-specific configurations.
func classifyQcomPins(rows []QcomPinRow) *QcomBoardConfig {
	cfg := &QcomBoardConfig{}
	macroSeen := make(map[string]bool)
	addMacro := func(name string, pin int, comment string) string {
		if name == "" {
			return ""
		}
		if !macroSeen[name] {
			macroSeen[name] = true
			cfg.Macros = append(cfg.Macros, QcomMacroDef{
				Name:    name,
				Value:   fmt.Sprintf("GPIO(%d)", pin),
				Comment: comment,
			})
		}
		return name
	}

	type qupAccum struct {
		proto QcomQUPProtocol
		texts []string
		pins  []int
	}
	qupMap := make(map[string]*qupAccum)
	var qupOrder []string

	for _, r := range rows {
		if shouldSkipQcomPin(r) {
			continue
		}

		if ctrl, se, proto, ok := matchQUPPin(r); ok {
			key := fmt.Sprintf("QUPV3_%d_SE%d", ctrl, se)
			acc, exists := qupMap[key]
			if !exists {
				acc = &qupAccum{proto: proto}
				qupMap[key] = acc
				qupOrder = append(qupOrder, key)
			}
			acc.texts = append(acc.texts, r.RawNetName+" "+r.EffectiveNet+" "+r.FunctionDesc)
			acc.pins = append(acc.pins, r.Pin)
			continue
		}

		uNet := strings.ToUpper(r.EffectiveNet)
		uRaw := strings.ToUpper(r.RawNetName)
		uCombined := uNet + " " + uRaw
		uFunc := strings.ToUpper(r.FunctionDesc)

		switch {
		// --- Bootblock / ChromeOS GPIOs ---
		case strings.Contains(uCombined, "KYBD_INT") || strings.Contains(uCombined, "EC_KBD_INT") ||
			(strings.Contains(uFunc, "INTERRUPT") && strings.Contains(uFunc, "FROM EC")):
			cfg.ECIntMacro = addMacro("GPIO_AP_EC_INT", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "GSC_AP_INT") || strings.Contains(uCombined, "GSC_SOC_INT"):
			cfg.GSCIntMacro = addMacro("GPIO_GSC_AP_INT", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "EN_PWR_FP") || (strings.Contains(uFunc, "FINGERPRINT") && strings.Contains(uFunc, "POWER ENABLE")):
			cfg.FPEnRailsMacro = addMacro("GPIO_EN_FP_RAILS", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "FP_RESET") || strings.Contains(uCombined, "FP_RST"):
			cfg.FPRstMacro = addMacro("GPIO_FP_RST_L", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "FP_FW_UP_STRAP") || strings.Contains(uCombined, "FP_BOOT0") || strings.Contains(uCombined, "FPMCU_BOOT0"):
			cfg.FPBoot0Macro = addMacro("GPIO_FPMCU_BOOT0", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "FPMCU_INT") || strings.Contains(uCombined, "FP_TO_AP_IRQ"):
			cfg.FPIntMacro = addMacro("GPIO_FPMCU_INT", r.Pin, r.EffectiveNet)

		// --- Display / eDP ---
		case strings.Contains(uCombined, "EDP_REG_EN") || strings.Contains(uCombined, "PANEL_POWER"):
			cfg.PanelPowerMacro = addMacro("GPIO_PANEL_POWER_ON", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "EDP0_HPD") || strings.Contains(uCombined, "EDP0_HOT_PLUG"):
			cfg.PanelHPDMacro = addMacro("GPIO_PANEL_HPD", r.Pin, r.EffectiveNet)

		// --- Lid & SD Card ---
		case strings.Contains(uCombined, "LID_INT") || strings.Contains(uCombined, "LID_OPEN"):
			m := addMacro("GPIO_LID_OPEN_S3", r.Pin, r.EffectiveNet)
			cfg.LidOpenMacro = m
			cfg.EarlyInputs = append(cfg.EarlyInputs, QcomEarlyGPIOEntry{
				MacroName: m,
				Pin:       r.Pin,
				NetName:   r.EffectiveNet,
				State:     EarlyGPIOInput,
			})

		case strings.Contains(uCombined, "SD_CARD_DET") || strings.Contains(uCombined, "SD_CD"):
			m := addMacro("GPIO_SD_CD_L", r.Pin, r.EffectiveNet)
			cfg.SDCardMacro = m
			cfg.EarlyInputs = append(cfg.EarlyInputs, QcomEarlyGPIOEntry{
				MacroName: m,
				Pin:       r.Pin,
				NetName:   r.EffectiveNet,
				State:     EarlyGPIOInput,
			})

		// --- USB Type-C1 Power & Retimer ---
		case strings.Contains(uCombined, "USB4_1_PWR_3P3_EN") || strings.Contains(uCombined, "USB1_PWR_3P3_EN"):
			cfg.USBC1En3P3Macro = addMacro("GPIO_USB_C1_EN_PP3300", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "USB4_1_PWR_1P8_EN") || strings.Contains(uCombined, "USB1_PWR_1P8_EN"):
			cfg.USBC1En1P8Macro = addMacro("GPIO_USB_C1_EN_PP1800", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "USB4_1_PWR_0P9_EN") || strings.Contains(uCombined, "USB1_PWR_1P15_EN"):
			cfg.USBC1En0P9Macro = addMacro("GPIO_USB_C1_EN_PP0900", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "USB4_1_RESET") || strings.Contains(uCombined, "USB1_RESET"):
			cfg.USBC1RetimerRst = addMacro("GPIO_USB_C1_RETIMER_RESET_L", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "USB4_0_PD_DISABLE") || strings.Contains(uCombined, "USB0_PD_DISABLE"):
			addMacro("GPIO_USB_C0_EN_L", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "USB4_1_PD_DISABLE") || strings.Contains(uCombined, "USB1_PD_DISABLE"):
			addMacro("GPIO_USB_C1_EN_L", r.Pin, r.EffectiveNet)

		// --- USB Camera & eUSB Repeaters ---
		case strings.Contains(uCombined, "EUSB5_RESET"):
			cfg.USBCamRstMacro = addMacro("GPIO_USB_CAM_RESET_L", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "CAM_PM_EM_EN"):
			cfg.USBCamEnMacro = addMacro("GPIO_USB_CAM_ENABLE", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "EUSB3_RESET") || strings.Contains(uCombined, "EUSB6_RESET"):
			m := addMacro(sanitizeNetToMacro(r.EffectiveNet), r.Pin, r.EffectiveNet)
			cfg.EUSBRepeaterResets = append(cfg.EUSBRepeaterResets, m)

		// --- NVMe / PCIe ---
		case strings.Contains(uCombined, "NVME_REG_EN"):
			addMacro("GPIO_NVME_REG_EN", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "NVME_PLN"):
			addMacro("GPIO_NVME_PLN_L", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "PCIE6A_RST") || strings.Contains(uCombined, "PCIE6_RST"):
			addMacro("GPIO_PCIE6A_RST_L", r.Pin, r.EffectiveNet)

		// --- Board ID & RAM ID Straps (boardid.c) ---
		case strings.Contains(uCombined, "BOARD_ID"):
			m := addMacro(sanitizeNetToMacro(r.EffectiveNet), r.Pin, r.EffectiveNet)
			cfg.BoardIDMacros = append(cfg.BoardIDMacros, m)

		case strings.Contains(uCombined, "RAM_ID"):
			m := addMacro(sanitizeNetToMacro(r.EffectiveNet), r.Pin, r.EffectiveNet)
			cfg.RamIDMacros = append(cfg.RamIDMacros, m)

		// --- Audio SoundWire / I2S & EGPIO ---
		case strings.Contains(uCombined, "SNDW_0_SCL"):
			cfg.SndwSCLMacro = addMacro("GPIO_SNDW_0_SCL", r.Pin, r.EffectiveNet)
			cfg.SndwSCLFunc = 4
			cfg.SndwSCLFuncNote = fmt.Sprintf("4=%s:WSA_SWR_CLK", r.EGPIOEnable)

		case strings.Contains(uCombined, "SNDW_0_SDA"):
			cfg.SndwSDAMacro = addMacro("GPIO_SNDW_0_SDA", r.Pin, r.EffectiveNet)
			cfg.SndwSDAFunc = 3
			cfg.SndwSDAFuncNote = fmt.Sprintf("3=%s:WSA_SWR_DATA", r.EGPIOEnable)

		case strings.Contains(uCombined, "WSA0_EN") || strings.Contains(uCombined, "AMP_ENABLE"):
			cfg.SndwAmp0EnMacro = addMacro("GPIO_SNDW_AMP_0_ENABLE", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "WSA1_EN"):
			cfg.SndwAmp1EnMacro = addMacro("GPIO_SNDW_AMP_1_ENABLE", r.Pin, r.EffectiveNet)

		case strings.Contains(uCombined, "MI2S") || strings.Contains(uCombined, "I2S"):
			m := addMacro(sanitizeNetToMacro(r.EffectiveNet), r.Pin, r.EffectiveNet)
			cfg.I2SPins = append(cfg.I2SPins, m)

		// --- Remaining Inputs & Active-Low Resets -> early_gpio.c ---
		default:
			if isEarlyResetOutput(uCombined, uFunc) {
				m := addMacro(sanitizeNetToMacro(r.EffectiveNet), r.Pin, r.EffectiveNet)
				cfg.EarlyOutputs = append(cfg.EarlyOutputs, QcomEarlyGPIOEntry{
					MacroName: m,
					Pin:       r.Pin,
					NetName:   r.EffectiveNet,
					State:     EarlyGPIOOutputLow,
				})
			} else if isEarlyInputPin(uCombined, uFunc, r.DefaultPull) {
				m := addMacro(sanitizeNetToMacro(r.EffectiveNet), r.Pin, r.EffectiveNet)
				cfg.EarlyInputs = append(cfg.EarlyInputs, QcomEarlyGPIOEntry{
					MacroName: m,
					Pin:       r.Pin,
					NetName:   r.EffectiveNet,
					State:     EarlyGPIOInput,
				})
			}
		}
	}

	// Sort EarlyInputs and EarlyOutputs by pin number for deterministic output.
	sort.Slice(cfg.EarlyInputs, func(i, j int) bool { return cfg.EarlyInputs[i].Pin < cfg.EarlyInputs[j].Pin })
	sort.Slice(cfg.EarlyOutputs, func(i, j int) bool { return cfg.EarlyOutputs[i].Pin < cfg.EarlyOutputs[j].Pin })

	// Finalize QUP buses (require at least 2 pins to form a valid bus).
	for _, key := range qupOrder {
		acc := qupMap[key]
		if len(acc.pins) < 2 {
			continue
		}
		combined := strings.Join(acc.texts, " ")
		role, mode, comment := classifyQUPBusRole(acc.proto, combined)
		cfg.QUPBuses = append(cfg.QUPBuses, QcomQUPBus{
			EnumName: key,
			Protocol: acc.proto,
			Mode:     mode,
			Role:     role,
			Comment:  comment,
		})
	}

	return cfg
}

func isEarlyResetOutput(uCombined, uFunc string) bool {
	if strings.Contains(uCombined, "RESET") || strings.Contains(uCombined, "_RST") {
		return true
	}
	if strings.Contains(uFunc, "RESET") {
		return true
	}
	return false
}

func isEarlyInputPin(uCombined, uFunc, defaultPull string) bool {
	if defaultPull != "PD" {
		return false
	}
	if strings.Contains(uCombined, "INT") || strings.Contains(uCombined, "IRQ") ||
		strings.Contains(uCombined, "WAKE") || strings.Contains(uCombined, "WP_OD") ||
		strings.Contains(uCombined, "LID_CLOSE") || strings.Contains(uCombined, "USB_OC") ||
		strings.Contains(uCombined, "FAULT") || strings.Contains(uFunc, "INTERRUPT") ||
		strings.Contains(uFunc, "WAKE") || strings.Contains(uFunc, "WRITE PROTECT") {
		return true
	}
	return false
}

// generateQcomCode writes the single consolidated C file with stage-delineated sections.
func generateQcomCode(cfg *QcomBoardConfig, w io.Writer) {
	fmt.Fprintln(w, "/* SPDX-License-Identifier: GPL-2.0-only */")
	fmt.Fprintln(w, "/* Generated by firmweaver coreboot_gpio --soc=qcom */")
	fmt.Fprintln(w, "")

	writeSectionBoardH(cfg, w)
	writeSectionEarlyGPIO(cfg, w)
	writeSectionBoardID(cfg, w)
	writeSectionChromeOS(cfg, w)
	writeSectionBootblock(cfg, w)
	writeSectionRomstage(cfg, w)
	writeSectionMainboard(cfg, w)
}

func writeSectionBoardH(cfg *QcomBoardConfig, w io.Writer) {
	fmt.Fprintln(w, "/* ========================================================================== *")
	fmt.Fprintln(w, " * Section 1: board.h - Symbolic GPIO Definitions")
	fmt.Fprintln(w, " * ========================================================================== */")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "#include <gpio.h>")
	fmt.Fprintln(w, "")

	for _, m := range cfg.Macros {
		fmt.Fprintf(w, "#define %-32s %s\t/* %s */\n", m.Name, m.Value, m.Comment)
		if m.Name == cfg.SndwSCLMacro && cfg.SndwSCLFunc > 0 {
			fmt.Fprintf(w, "#define %-32s %d\t\t/* %s */\n", "GPIO_FUN_SNDW_0_SCL", cfg.SndwSCLFunc, cfg.SndwSCLFuncNote)
		}
		if m.Name == cfg.SndwSDAMacro && cfg.SndwSDAFunc > 0 {
			fmt.Fprintf(w, "#define %-32s %d\t\t/* %s */\n", "GPIO_FUN_SNDW_0_SDA", cfg.SndwSDAFunc, cfg.SndwSDAFuncNote)
		}
	}
	fmt.Fprintln(w, "")
}

func writeSectionEarlyGPIO(cfg *QcomBoardConfig, w io.Writer) {
	fmt.Fprintln(w, "/* ========================================================================== *")
	fmt.Fprintln(w, " * Section 2: early_gpio.c - Early Romstage Default GPIO Configuration Table")
	fmt.Fprintln(w, " * ========================================================================== */")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "enum gpio_state {")
	fmt.Fprintln(w, "\tGPIO_STATE_INPUT,")
	fmt.Fprintln(w, "\tGPIO_STATE_OUTPUT_LOW,")
	fmt.Fprintln(w, "\tGPIO_STATE_OUTPUT_HIGH,")
	fmt.Fprintln(w, "};")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "struct gpio_config {")
	fmt.Fprintln(w, "\tgpio_t pad;")
	fmt.Fprintln(w, "\tenum gpio_state state;")
	fmt.Fprintln(w, "};")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "#define GPIO_CFG_IN(pad_)       { .pad = (pad_), .state = GPIO_STATE_INPUT }")
	fmt.Fprintln(w, "#define GPIO_CFG_OUT_LOW(pad_)  { .pad = (pad_), .state = GPIO_STATE_OUTPUT_LOW }")
	fmt.Fprintln(w, "#define GPIO_CFG_OUT_HIGH(pad_) { .pad = (pad_), .state = GPIO_STATE_OUTPUT_HIGH }")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static const struct gpio_config default_gpio_table[] = {")
	if len(cfg.EarlyInputs) > 0 {
		fmt.Fprintln(w, "\t/*")
		fmt.Fprintln(w, "\t * INPUTS: Interrupts, Wakes, Detects, Faults, Open-Drain signals")
		fmt.Fprintln(w, "\t * Configured as inputs with No Pull (NP) to disable default internal PD.")
		fmt.Fprintln(w, "\t */")
		for _, in := range cfg.EarlyInputs {
			fmt.Fprintf(w, "\t%s(%s),\t/* GPIO(%d): %s */\n", in.State, in.MacroName, in.Pin, in.NetName)
		}
	}
	if len(cfg.EarlyOutputs) > 0 {
		if len(cfg.EarlyInputs) > 0 {
			fmt.Fprintln(w, "")
		}
		fmt.Fprintln(w, "\t/*")
		fmt.Fprintln(w, "\t * OUTPUTS: Active-low peripheral resets held low during early boot")
		fmt.Fprintln(w, "\t */")
		for _, out := range cfg.EarlyOutputs {
			fmt.Fprintf(w, "\t%s(%s),\t/* GPIO(%d): %s */\n", out.State, out.MacroName, out.Pin, out.NetName)
		}
	}
	fmt.Fprintln(w, "};")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "void configure_gpios_defaults(void)")
	fmt.Fprintln(w, "{")
	fmt.Fprintln(w, "\tfor (size_t i = 0; i < ARRAY_SIZE(default_gpio_table); i++) {")
	fmt.Fprintln(w, "\t\tconst struct gpio_config *cfg = &default_gpio_table[i];")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "\t\tswitch (cfg->state) {")
	fmt.Fprintln(w, "\t\tcase GPIO_STATE_INPUT:")
	fmt.Fprintln(w, "\t\t\tgpio_input(cfg->pad);")
	fmt.Fprintln(w, "\t\t\tbreak;")
	fmt.Fprintln(w, "\t\tcase GPIO_STATE_OUTPUT_LOW:")
	fmt.Fprintln(w, "\t\t\tgpio_output(cfg->pad, 0);")
	fmt.Fprintln(w, "\t\t\tbreak;")
	fmt.Fprintln(w, "\t\tcase GPIO_STATE_OUTPUT_HIGH:")
	fmt.Fprintln(w, "\t\t\tgpio_output(cfg->pad, 1);")
	fmt.Fprintln(w, "\t\t\tbreak;")
	fmt.Fprintln(w, "\t\t}")
	fmt.Fprintln(w, "\t}")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
}

func writeSectionBoardID(cfg *QcomBoardConfig, w io.Writer) {
	if len(cfg.BoardIDMacros) == 0 && len(cfg.RamIDMacros) == 0 {
		return
	}
	fmt.Fprintln(w, "/* ========================================================================== *")
	fmt.Fprintln(w, " * Section 2b: boardid.c - Board ID & RAM ID Strapping Pins")
	fmt.Fprintln(w, " * ========================================================================== */")
	fmt.Fprintln(w, "")
	if len(cfg.BoardIDMacros) > 0 {
		fmt.Fprintln(w, "uint32_t board_id(void)")
		fmt.Fprintln(w, "{")
		fmt.Fprintln(w, "\tstatic uint32_t id = UNDEFINED_STRAPPING_ID;")
		fmt.Fprintln(w, "\tif (id != UNDEFINED_STRAPPING_ID)")
		fmt.Fprintln(w, "\t\treturn id;")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "\tconst gpio_t pins[] = {")
		for i, m := range cfg.BoardIDMacros {
			fmt.Fprintf(w, "\t\t[%d] = %s,\n", i, m)
		}
		fmt.Fprintln(w, "\t};")
		fmt.Fprintln(w, "\tid = gpio_base3_value(pins, ARRAY_SIZE(pins));")
		fmt.Fprintln(w, "\treturn id;")
		fmt.Fprintln(w, "}")
		fmt.Fprintln(w, "")
	}
	if len(cfg.RamIDMacros) > 0 {
		fmt.Fprintln(w, "uint32_t ram_code(void)")
		fmt.Fprintln(w, "{")
		fmt.Fprintln(w, "\tstatic uint32_t id = UNDEFINED_STRAPPING_ID;")
		fmt.Fprintln(w, "\tif (id != UNDEFINED_STRAPPING_ID)")
		fmt.Fprintln(w, "\t\treturn id;")
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "\tconst gpio_t pins[] = {")
		for i, m := range cfg.RamIDMacros {
			fmt.Fprintf(w, "\t\t[%d] = %s,\n", i, m)
		}
		fmt.Fprintln(w, "\t};")
		fmt.Fprintln(w, "\tid = gpio_base2_value(pins, ARRAY_SIZE(pins));")
		fmt.Fprintln(w, "\treturn id;")
		fmt.Fprintln(w, "}")
		fmt.Fprintln(w, "")
	}
}

func writeSectionChromeOS(cfg *QcomBoardConfig, w io.Writer) {
	fmt.Fprintln(w, "/* ========================================================================== *")
	fmt.Fprintln(w, " * Section 3: chromeos.c - ChromeOS GPIO Setup & Coreboot Table Handoff")
	fmt.Fprintln(w, " * ========================================================================== */")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "void setup_chromeos_gpios(void)")
	fmt.Fprintln(w, "{")
	if cfg.ECIntMacro != "" {
		fmt.Fprintln(w, "\tif (CONFIG(EC_GOOGLE_CHROMEEC))")
		fmt.Fprintf(w, "\t\tgpio_input_pullup(%s);\n", cfg.ECIntMacro)
	}
	if cfg.GSCIntMacro != "" {
		if cfg.ECIntMacro != "" {
			fmt.Fprintln(w, "")
		}
		fmt.Fprintln(w, "\tif (CONFIG(TPM_GOOGLE_TI50))")
		fmt.Fprintf(w, "\t\tgpio_input_irq(%s, IRQ_TYPE_RISING_EDGE, GPIO_PULL_UP);\n", cfg.GSCIntMacro)
	}
	if cfg.FPRstMacro != "" || cfg.FPEnRailsMacro != "" || cfg.FPBoot0Macro != "" || cfg.FPIntMacro != "" {
		fmt.Fprintln(w, "")
		fmt.Fprintln(w, "\tif (CONFIG(MAINBOARD_HAS_FINGERPRINT)) {")
		if cfg.FPRstMacro != "" {
			fmt.Fprintf(w, "\t\tgpio_output(%s, 0);\n", cfg.FPRstMacro)
		}
		if cfg.FPEnRailsMacro != "" {
			fmt.Fprintf(w, "\t\tgpio_output(%s, 0);\n", cfg.FPEnRailsMacro)
		}
		if cfg.FPBoot0Macro != "" || cfg.FPIntMacro != "" {
			fmt.Fprintln(w, "\t\tif (CONFIG(MAINBOARD_HAS_FINGERPRINT_VIA_SPI)) {")
			if cfg.FPBoot0Macro != "" {
				fmt.Fprintf(w, "\t\t\tgpio_output(%s, 0);\n", cfg.FPBoot0Macro)
			}
			if cfg.FPIntMacro != "" {
				fmt.Fprintf(w, "\t\t\tgpio_input_irq(%s, IRQ_TYPE_LEVEL, GPIO_PULL_UP);\n", cfg.FPIntMacro)
			}
			fmt.Fprintln(w, "\t\t}")
		}
		fmt.Fprintln(w, "\t}")
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "void fill_lb_gpios(struct lb_gpios *gpios)")
	fmt.Fprintln(w, "{")
	fmt.Fprintln(w, "\tconst struct lb_gpio chromeos_gpios[] = {")
	if cfg.ECIntMacro != "" {
		fmt.Fprintln(w, "#if CONFIG(EC_GOOGLE_CHROMEEC)")
		fmt.Fprintf(w, "\t\t{%s.addr, ACTIVE_LOW, gpio_get(%s),\n\t\t\t\"EC interrupt\"},\n", cfg.ECIntMacro, cfg.ECIntMacro)
		fmt.Fprintln(w, "#endif")
	}
	if cfg.PanelPowerMacro != "" {
		fmt.Fprintf(w, "\t\t{%s.addr, ACTIVE_HIGH, gpio_get(%s),\n\t\t\t\"Panel VDD en\"},\n", cfg.PanelPowerMacro, cfg.PanelPowerMacro)
	}
	if cfg.GSCIntMacro != "" {
		fmt.Fprintln(w, "#if CONFIG(TPM_GOOGLE_TI50)")
		fmt.Fprintf(w, "\t\t{%s.addr, ACTIVE_HIGH, gpio_get(%s),\n\t\t\t\"TPM interrupt\"},\n", cfg.GSCIntMacro, cfg.GSCIntMacro)
		fmt.Fprintln(w, "#endif")
	}
	if cfg.SndwAmp0EnMacro != "" {
		fmt.Fprintf(w, "\t\t{%s.addr, ACTIVE_HIGH, gpio_get(%s),\n\t\t\t\"Speaker 0 enable\"},\n", cfg.SndwAmp0EnMacro, cfg.SndwAmp0EnMacro)
	}
	if cfg.SndwAmp1EnMacro != "" {
		fmt.Fprintf(w, "\t\t{%s.addr, ACTIVE_HIGH, gpio_get(%s),\n\t\t\t\"Speaker 1 enable\"},\n", cfg.SndwAmp1EnMacro, cfg.SndwAmp1EnMacro)
	}
	if cfg.SDCardMacro != "" {
		fmt.Fprintln(w, "#if CONFIG(MAINBOARD_HAS_SD_CONTROLLER)")
		fmt.Fprintf(w, "\t\t{%s.addr, ACTIVE_LOW, gpio_get(%s),\n\t\t\t\"SD card detect\"},\n", cfg.SDCardMacro, cfg.SDCardMacro)
		fmt.Fprintln(w, "#endif")
	}
	fmt.Fprintln(w, "\t};")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "\tlb_add_gpios(gpios, chromeos_gpios, ARRAY_SIZE(chromeos_gpios));")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
}

func writeSectionBootblock(cfg *QcomBoardConfig, w io.Writer) {
	fmt.Fprintln(w, "/* ========================================================================== *")
	fmt.Fprintln(w, " * Section 4: bootblock.c - Bootblock GPIO & Early QUP Bus Initialization")
	fmt.Fprintln(w, " * ========================================================================== */")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "void bootblock_mainboard_init(void)")
	fmt.Fprintln(w, "{")
	fmt.Fprintln(w, "\tsetup_chromeos_gpios();")

	for _, b := range cfg.QUPBuses {
		switch b.Role {
		case QUPRoleBootblockTPM:
			fmt.Fprintln(w, "")
			fmt.Fprintln(w, "\tif (CONFIG(I2C_TPM))")
			fmt.Fprintf(w, "\t\ti2c_init(%s, I2C_SPEED_FAST); /* %s */\n", b.EnumName, b.Comment)
		case QUPRoleBootblockECSPI:
			fmt.Fprintln(w, "")
			fmt.Fprintln(w, "\tif (CONFIG(EC_GOOGLE_CHROMEEC))")
			fmt.Fprintf(w, "\t\tqup_spi_init(%s, 1010 * KHz); /* %s */\n", b.EnumName, b.Comment)
		}
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
}

func writeSectionRomstage(cfg *QcomBoardConfig, w io.Writer) {
	fmt.Fprintln(w, "/* ========================================================================== *")
	fmt.Fprintln(w, " * Section 5: romstage.c - Romstage Peripheral Power & Reset Sequencing")
	fmt.Fprintln(w, " * ========================================================================== */")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static void early_setup_usb_typec(void)")
	fmt.Fprintln(w, "{")
	if cfg.USBC1RetimerRst != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 0);\n", cfg.USBC1RetimerRst)
	}
	if cfg.USBC1En3P3Macro != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 0);\n", cfg.USBC1En3P3Macro)
	}
	if cfg.USBC1En1P8Macro != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 0);\n", cfg.USBC1En1P8Macro)
	}
	if cfg.USBC1En0P9Macro != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 0);\n", cfg.USBC1En0P9Macro)
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static void edp_configure_gpios(void)")
	fmt.Fprintln(w, "{")
	if cfg.PanelPowerMacro != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 1);\n", cfg.PanelPowerMacro)
	}
	if cfg.PanelHPDMacro != "" {
		fmt.Fprintf(w, "\tgpio_input(%s);\n", cfg.PanelHPDMacro)
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static void mainboard_setup_peripherals_early(void)")
	fmt.Fprintln(w, "{")
	fmt.Fprintln(w, "\tconfigure_gpios_defaults();")
	fmt.Fprintln(w, "\tedp_configure_gpios();")
	if cfg.LidOpenMacro != "" {
		fmt.Fprintf(w, "\tgpio_input(%s);\n", cfg.LidOpenMacro)
	}
	fmt.Fprintln(w, "\tearly_setup_usb_typec();")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static void mainboard_setup_peripherals_late(int mode)")
	fmt.Fprintln(w, "{")
	if cfg.USBC1En3P3Macro != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 1);\n", cfg.USBC1En3P3Macro)
	}
	if cfg.FPEnRailsMacro != "" {
		fmt.Fprintln(w, "\tif (CONFIG(MAINBOARD_HAS_FINGERPRINT)) {")
		fmt.Fprintln(w, "\t\tif (mode == LB_BOOT_MODE_NORMAL || mode == LB_BOOT_MODE_NO_BATTERY)")
		fmt.Fprintf(w, "\t\t\tgpio_output(%s, 1);\n", cfg.FPEnRailsMacro)
		fmt.Fprintln(w, "\t}")
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "void platform_romstage_postram(void)")
	fmt.Fprintln(w, "{")
	if cfg.USBC1En1P8Macro != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 1);\n", cfg.USBC1En1P8Macro)
	}
	if cfg.USBC1En1P8Macro != "" && cfg.USBC1En0P9Macro != "" {
		fmt.Fprintln(w, "\tmdelay(1);")
	}
	if cfg.USBC1En0P9Macro != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 1);\n", cfg.USBC1En0P9Macro)
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
}

func writeSectionMainboard(cfg *QcomBoardConfig, w io.Writer) {
	fmt.Fprintln(w, "/* ========================================================================== *")
	fmt.Fprintln(w, " * Section 6: mainboard.c - Ramstage Peripherals, Audio EGPIO & QUP SE FW")
	fmt.Fprintln(w, " * ========================================================================== */")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static void enable_usb_camera(void)")
	fmt.Fprintln(w, "{")
	if cfg.USBCamRstMacro != "" || cfg.USBCamEnMacro != "" {
		fmt.Fprintln(w, "\tif (CONFIG(MAINBOARD_HAS_CAMERA_VIA_USB)) {")
		if cfg.USBCamRstMacro != "" {
			fmt.Fprintf(w, "\t\tgpio_output(%s, 1);\n", cfg.USBCamRstMacro)
		}
		if cfg.USBCamEnMacro != "" {
			fmt.Fprintf(w, "\t\tgpio_output(%s, 1);\n", cfg.USBCamEnMacro)
		}
		if cfg.USBCamEnMacro != "" {
			fmt.Fprintln(w, "\t} else {")
			fmt.Fprintf(w, "\t\tgpio_output(%s, 0);\n", cfg.USBCamEnMacro)
		}
		fmt.Fprintln(w, "\t}")
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static void setup_usb_typec(void)")
	fmt.Fprintln(w, "{")
	if cfg.USBC1RetimerRst != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 1);\n", cfg.USBC1RetimerRst)
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static void setup_audio_gpios(void)")
	fmt.Fprintln(w, "{")
	if cfg.SndwAmp0EnMacro != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 0);\n", cfg.SndwAmp0EnMacro)
	}
	if cfg.SndwAmp1EnMacro != "" {
		fmt.Fprintf(w, "\tgpio_output(%s, 0);\n", cfg.SndwAmp1EnMacro)
	}
	if cfg.SndwSCLMacro != "" {
		fmt.Fprintf(w, "\tgpio_configure_no_egpio(%s, GPIO_FUN_SNDW_0_SCL,\n\t\tGPIO_NO_PULL, GPIO_16MA, GPIO_OUTPUT);\n", cfg.SndwSCLMacro)
	}
	if cfg.SndwSDAMacro != "" {
		fmt.Fprintf(w, "\tgpio_configure_no_egpio(%s, GPIO_FUN_SNDW_0_SDA,\n\t\tGPIO_NO_PULL, GPIO_16MA, GPIO_OUTPUT);\n", cfg.SndwSDAMacro)
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static void load_qc_se_firmware_early(void)")
	fmt.Fprintln(w, "{")
	for _, b := range cfg.QUPBuses {
		if b.Role == QUPRoleRamstageEarlyADSP {
			fmt.Fprintf(w, "\tqupv3_se_fw_load_and_init(%s, %s, %s); /* %s */\n", b.EnumName, b.Protocol, b.Mode, b.Comment)
		}
	}
	fmt.Fprintln(w, "\tgpi_firmware_load(QUP_0_GSI_BASE);")
	fmt.Fprintln(w, "\tgpi_firmware_load(QUP_1_GSI_BASE);")
	fmt.Fprintln(w, "\tgpi_firmware_load(QUP_2_GSI_BASE);")
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "static void load_qc_se_firmware_late(void)")
	fmt.Fprintln(w, "{")
	var retimerBuses []QcomQUPBus
	for _, b := range cfg.QUPBuses {
		if b.Role == QUPRoleRamstageUSBC0RetimerI2C || b.Role == QUPRoleRamstageUSBC1RetimerI2C {
			retimerBuses = append(retimerBuses, b)
		}
	}

	for _, b := range cfg.QUPBuses {
		switch b.Role {
		case QUPRoleRamstageDebugUART:
			fmt.Fprintln(w, "\tif (!CONFIG(CONSOLE_SERIAL))")
			fmt.Fprintf(w, "\t\tqupv3_se_fw_load_and_init(%s, %s, %s); /* %s */\n", b.EnumName, b.Protocol, b.Mode, b.Comment)
		case QUPRoleRamstageTouchI2C, QUPRoleRamstageBTUART, QUPRoleRamstageTrackpadI2C, QUPRoleRamstageAppsI2C:
			fmt.Fprintf(w, "\tqupv3_se_fw_load_and_init(%s, %s, %s); /* %s */\n", b.EnumName, b.Protocol, b.Mode, b.Comment)
		case QUPRoleRamstageUSBARetimerI2C:
			fmt.Fprintln(w, "\tif (!CONFIG(MAINBOARD_NO_USB_A_PORT))")
			fmt.Fprintf(w, "\t\tqupv3_se_fw_load_and_init(%s, %s, %s); /* %s */\n", b.EnumName, b.Protocol, b.Mode, b.Comment)
		case QUPRoleRamstageFPSPI:
			fmt.Fprintln(w, "\tif (CONFIG(MAINBOARD_HAS_FINGERPRINT_VIA_SPI))")
			fmt.Fprintf(w, "\t\tqupv3_se_fw_load_and_init(%s, %s, %s); /* %s */\n", b.EnumName, b.Protocol, b.Mode, b.Comment)
		}
	}

	if len(retimerBuses) > 0 {
		fmt.Fprintln(w, "\tif (CONFIG(MAINBOARD_HAS_PS8820_RETIMER)) {")
		for _, b := range retimerBuses {
			fmt.Fprintf(w, "\t\ti2c_init(%s, I2C_SPEED_FAST); /* %s */\n", b.EnumName, b.Comment)
		}
		fmt.Fprintln(w, "\t} else {")
		for _, b := range retimerBuses {
			fmt.Fprintf(w, "\t\tqupv3_se_fw_load_and_init(%s, %s, %s); /* %s */\n", b.EnumName, b.Protocol, b.Mode, b.Comment)
		}
		fmt.Fprintln(w, "\t}")
	}
	fmt.Fprintln(w, "}")
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "void mainboard_soc_init(void)")
	fmt.Fprintln(w, "{")
	fmt.Fprintln(w, "\tsetup_usb_typec();")
	fmt.Fprintln(w, "\tenable_usb_camera();")
	if cfg.FPRstMacro != "" {
		fmt.Fprintln(w, "\tif (CONFIG(MAINBOARD_HAS_FINGERPRINT))")
		fmt.Fprintf(w, "\t\tgpio_output(%s, 1);\n", cfg.FPRstMacro)
	}
	fmt.Fprintln(w, "\tsetup_audio_gpios();")
	for _, rst := range cfg.EUSBRepeaterResets {
		fmt.Fprintf(w, "\tgpio_output(%s, 1);\n", rst)
	}
	fmt.Fprintln(w, "}")
}
