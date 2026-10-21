# Qualcomm AP GPIO Configuration Generator

This document describes how the `firmweaver` Qualcomm Application Processor (AP) GPIO configuration generator (`./firmweaver gpio --soc=qcom`) works, its key design considerations, and how it classifies or skips pins from a Qualcomm GPIO map spreadsheet.

---

## 1. Background: Why Qualcomm Differs from Intel

### Intel GPIO Configuration
On **Intel** mainboards:
1. **Explicit Pin Configuration Columns**: The Intel GPIO spreadsheet template includes explicit columns for pad mode (`Pmode`), direction (`Gpio Pin Direction`), initial output value (`Gpio Output State`), and reset configuration (`PadRstCfg`).
2. **Single Declarative Table Format**: Intel GPIOs in coreboot are configured using declarative `PAD_CFG_*` macros in a `struct pad_config` table. Even when Intel mainboards configure GPIOs across multiple boot stages (such as configuring early pads in `bootblock` or `romstage` and the full set in `ramstage`), the early stage configuration is simply a copy-pasted subset of the same declarative `PAD_CFG_*` entries. As a result, the tool can generate a single declarative table for all GPIOs and leave it to the developer to copy the required subset for early stages—without needing boot-stage annotations in the spreadsheet.

### Qualcomm GPIO Configuration
On **Qualcomm** mainboards:
1. **Different Configuration Constructs Across Stages and Files**: Rather than replicating entries from a single declarative table across stages, Qualcomm mainboard code uses different C constructs depending on the pin's role and boot stage:
   - Symbolic `#define GPIO_<NAME> GPIO(<N>)` macros in `board.h`
   - A table-driven reset/input initialization array (`default_gpio_table[]`) in `early_gpio.c`
   - Strapping pin arrays (`gpio_base3_value()` / `gpio_base2_value()`) in `boardid.c`
   - Imperative GPIO API calls (`gpio_input_pullup()`, `gpio_input_irq()`, `gpio_output()`, `gpio_configure_no_egpio()`) and coreboot table handoff entries (`fill_lb_gpios()`) across `chromeos.c`, `bootblock.c`, `romstage.c`, and `mainboard.c`
   - QUPv3 Serial Engine bus initialization (`i2c_init()`, `qup_spi_init()`, `qupv3_se_fw_load_and_init()`) in `bootblock.c` and `mainboard.c`
2. **Missing Direction and Output State Columns in the Sheet**: While both Intel and Qualcomm GPIO spreadsheets share common columns (pad name, default reset pull, reference board net/function, target board net/function, and alternate functions), the Qualcomm GPIO spreadsheet template **does not contain explicit GPIO direction or initial output state columns**. Beyond the common hardware pinout columns, it only provides Qualcomm-specific low-power island ownership (`EGPIO enable` for `LPASS` and `SSC` islands).

---

## 2. Key Design Considerations

### 1. Heuristic-Based Generation on the Unmodified Sheet Template
Rather than requiring additional custom columns (`Direction`, `Initial Output State`, etc.) in the Qualcomm GPIO spreadsheet template, the generator consumes the **existing spreadsheet template as-is** and applies heuristics over net names, function descriptions, default reset pulls, QUPv3 alternate functions, and `EGPIO enable` values.
- **Rationale**: Modifying standard pinmap templates to add custom columns requires manual maintenance and is prone to drift when boot-stage sequencing evolves. Heuristic classification enables immediate code generation directly from existing board sheets during initial bringup, producing a baseline configuration that developers can review and integrate.

### 2. Automatic Target Board Column Detection
Qualcomm GPIO spreadsheets typically include reference schematic columns (containing `"CRD"` or `"Reference"` in the header) alongside the target board's columns. Many sheets also contain both an initial board net name column (`<Board> Net Name`) and a final schematic net name column (`<Board> Schematic net name`).
- **Column Selection Rules**:
  1. Any column header containing `"CRD"`, `"RVP"`, or `"Reference"` (case-insensitive) is excluded as a reference board column.
  2. Among the remaining board columns, `<Board> Schematic net name`, `<Board> Net Name`, and `<Board> Function` are selected automatically without requiring extra CLI flags.
  3. **Disconnect Resolution**: If *either* board net name column (or the board function column) explicitly marks a pin as disconnected (`NC`, `TP (...)`, `Not connected`, `UNUSED`), the pin is treated as unpopulated—preventing leftover template nets in one column from generating false configurations when the other column marks the pin `NC`. Otherwise, `<Board> Schematic net name` is used as the primary net name, while `<Board> Net Name` is also checked for peripheral subsystem keywords (for example, when `<Board> Net Name` retains `I2C_2_USBC0_SDA` and `<Board> Schematic net name` shortens it to `I2C_2_SDA`).

### 3. Single Consolidated Output File with Stage-Delineated Sections
To keep CLI usage simple and consistent (`--output=<file>` or `stdout`), the generator emits a **single consolidated C file** partitioned into clearly delineated sections corresponding to the target coreboot mainboard files:
- **Section 1 (`board.h`)**: Symbolic `#define GPIO_<NAME> GPIO(<N>)` macros (plus `GPIO_FUN_SNDW_0_SCL/SDA` mux macros).
- **Section 2 (`early_gpio.c`)**: `default_gpio_table[]` (`GPIO_CFG_IN`, `GPIO_CFG_OUT_LOW`, `GPIO_CFG_OUT_HIGH`) and `configure_gpios_defaults()`.
- **Section 2b (`boardid.c`)**: `board_id()` (`gpio_base3_value`) and `ram_code()` (`gpio_base2_value`) strapping arrays when `BOARD_ID*` / `RAM_ID*` pins are present.
- **Section 3 (`chromeos.c`)**: `setup_chromeos_gpios()` and `fill_lb_gpios()`.
- **Section 4 (`bootblock.c`)**: `bootblock_mainboard_init()` calling `setup_chromeos_gpios()`, TPM I2C `i2c_init()`, and EC SPI `qup_spi_init()`.
- **Section 5 (`romstage.c`)**: `early_setup_usb_typec()`, `edp_configure_gpios()`, `mainboard_setup_peripherals_early()`, `mainboard_setup_peripherals_late()`, and `platform_romstage_postram()`.
- **Section 6 (`mainboard.c`)**: `enable_usb_camera()`, `setup_usb_typec()`, `setup_audio_gpios()`, `load_qc_se_firmware_early()`, `load_qc_se_firmware_late()`, and `mainboard_soc_init()`.

### 4. Early Romstage Default GPIO Table (`early_gpio.c`)
When GPIO pads default to an internal pull-down (`PD`) at reset, connecting open-drain or externally pulled-up signals (interrupts, wakes, card/lid detects, write-protect) to those pads without reconfiguring them creates a voltage divider and causes unnecessary current leakage during boot. Similarly, active-low peripheral resets not sequenced in `bootblock` must be held low early.
- Following the standard Qualcomm mainboard `early_gpio.c` pattern in coreboot, the generator populates `default_gpio_table[]`:
  - **`GPIO_CFG_IN`**: Emitted for open-drain / externally pulled-up input signals (`*INT*`, `*IRQ*`, `*WAKE*`, `*WP_OD*`, `*LID_CLOSE*`, `*USB_OC*`, `*FAULT*`, `*SD_CARD_DET*`) whose default reset pull is `PD`, configuring them as inputs with `GPIO_NO_PULL`.
  - **`GPIO_CFG_OUT_LOW`**: Emitted for remaining active-low peripheral reset lines (`*RESET*`, `*_RST*`) to hold peripherals in reset during early boot.

### 5. Handling the `Boot configuration` Column
In Qualcomm GPIO spreadsheets, the `Boot configuration` column indicates pads that have an associated `BOOT_CONFIG[n]` hardware strap. Because some pins that share a pad with a boot strap are only sampled at power-on reset and operate as standard GPIOs at runtime (for example, a peripheral power enable sharing a pad with a `BOOT_CONFIG[n]` strap), pins are **not** skipped solely because the `Boot configuration` column is non-empty. Dedicated strap-only pins are instead identified by their net name (`BOOT_CONFIG*`, `FORCED_USB_BOOT`).

---

## 3. Pin Classification: Configured vs. Skipped Pins

### Configured Pins by Stage

| Subsystem / Role | Heuristic Match Criteria | Target Stage & Section |
| :--- | :--- | :--- |
| **EC Interrupt** | `*KYBD_INT*`, `*EC_KBD_INT*`, Function `"Interrupt ... from EC"` | `bootblock` (`chromeos.c`: `gpio_input_pullup` + `fill_lb_gpios`) |
| **GSC / TPM Interrupt** | `*GSC_AP_INT*`, `*GSC_SOC_INT*` | `bootblock` (`chromeos.c`: `gpio_input_irq` + `fill_lb_gpios`) |
| **Fingerprint (`FPMCU`)** | `EN_PWR_FP`, `FP_RESET*`, `FP_FW_UP_STRAP` / `FP_BOOT0`, `FPMCU_INT*` | `bootblock` (`chromeos.c`: hold reset/rails/boot0 low, config IRQ) -> `romstage` late (enable `GPIO_EN_FP_RAILS`) -> `ramstage` (deassert `GPIO_FP_RST_L`) |
| **Early QUP Buses (TPM & EC)** | `Alternate function 1..2` = `QUPx_SEy_Lz` + net `*GSC*`/`*TPM*` (I2C) or `*EC_SPI*` (SPI) | `bootblock` (`bootblock.c`: `i2c_init` / `qup_spi_init`) |
| **Display (eDP)** | `EDP_REG_EN` / `*PANEL_POWER*`, `EDP0_HPD` / `*EDP0_HOT_PLUG*` | `romstage` (`romstage.c`: `edp_configure_gpios()`) + `chromeos.c` (`fill_lb_gpios`) |
| **Lid Switch & SD Card Detect** | `LID_INT*` / `LID_OPEN*`, `SD_CARD_DET*` / `SD_CD*` | `romstage` (`early_gpio.c` `GPIO_CFG_IN` + `romstage.c`) + `chromeos.c` (`fill_lb_gpios`) |
| **USB Type-C1 Power & Retimer** | `USB4_1_PWR_3P3_EN`, `USB4_1_PWR_1P8_EN`, `USB4_1_PWR_0P9_EN`, `USB4_1_RESET#` | Multi-stage: `romstage` early (all `0`) -> `romstage` late (`3P3_EN` = `1`) -> `romstage` postram (`1P8_EN` = `1`, `mdelay(1)`, `0P9_EN` = `1`) -> `ramstage` (`RETIMER_RESET_L` = `1`) |
| **Board ID & RAM ID Straps** | `*BOARD_ID*`, `*RAM_ID*` | `boardid.c` (`board_id()` via `gpio_base3_value`, `ram_code()` via `gpio_base2_value`) |
| **Speaker Audio (Dev-Mode Beep)** | `EGPIO enable` = `LPASS_*` with `SNDW_0_SCL*`, `SNDW_0_SDA*`, `WSA0_EN`, `WSA1_EN`, or `*I2S*` / `*AMP_ENABLE*` | `ramstage` (`mainboard.c`: `setup_audio_gpios()` calling `gpio_configure_no_egpio` to hand off from LPASS to AP for `depthcharge` boot beep) + `chromeos.c` (`fill_lb_gpios`) |
| **USB Camera & eUSB Repeaters** | `EUSB5_RESET#`, `CAM_PM_EM_EN`, `EUSB3_RESET#`, `EUSB6_RESET#` | `ramstage` (`mainboard.c`: `enable_usb_camera()` & `mainboard_soc_init()`) |
| **Ramstage QUPv3 SE Firmware** | `Alternate function 1..2` = `QUPx_SEy_Lz` for ADSP/Charger I2C, Touch I2C, Trackpad I2C, USB-C/A Retimer I2C, Apps/eUSB I2C (`GSI`), FP SPI, BT UART, Debug UART | `ramstage` (`mainboard.c`: `load_qc_se_firmware_early()` and `load_qc_se_firmware_late()`) |
| **Open-Drain Inputs & Peripheral Resets** | Remaining `*INT*`, `*IRQ*`, `*WAKE*`, `*WP_OD*`, `*LID_CLOSE*`, `*USB_OC*`, `*FAULT*` (with default `PD`) and `*RESET*` / `*_RST*` | `romstage` (`early_gpio.c`: `default_gpio_table[]`) |

### Skipped Pins and Why Coreboot Mainboard Code Does Not Configure Them

| Skipped Category | Net / Column Pattern | Reason for Skipping in Mainboard GPIO Generator |
| :--- | :--- | :--- |
| **Disconnected / Test Points** | `NC`, `TP (...)`, `–`, empty, Function `"Not connected"` / `"UNUSED"` | Unpopulated on the board schematic. |
| **Hardware POR Boot Straps** | `BOOT_CONFIG*`, `FORCED_USB_BOOT` | Sampled by hardware at power-on reset before coreboot executes. |
| **Boot Flash QSPI** | `SOC_SPI_0_*`, `QSPI*` | Configured unconditionally by the common SoC QSPI driver (`quadspi_init()` in `src/soc/qualcomm/common/qspi.c`), not in mainboard code. |
| **SoC Reset / Throttling** | `RESOUT*`, `SYS_THROTTLE*` | Dedicated SoC reset and thermal throttling pins not driven as generic mainboard GPIOs. |
| **SSC Sensor Island** | `EGPIO enable` = `SSC_*` | Owned by the Sensor Low-Power Island (SSC) and initialized by sensor firmware in the OS. |
| **LPASS Non-Speaker Audio (WCD & DMIC)** | `EGPIO enable` = `LPASS_*` for `SNDW_0_CTX/CRX_*` (jack codec), `DMIC*` (digital mics), `SOC_SMB*` | `depthcharge` dev-mode beep only uses the speaker amplifiers (`WSA` / `I2S`). Headphone codec (`WCD`) and microphone (`DMIC`) buses remain in LPASS island mode for the OS kernel driver (note: discrete codec reset pins without `LPASS_*` EGPIO *are* held low in `early_gpio.c`). |
| **MIPI Camera ISP / CCI** | `CAM_MCLK*`, `CAM_AON_MCLK*`, `CCI_I2C*`, `IRCAM*`, `CAM_PRIVACY*`, `CAM0/4_RESET#` | Dedicated MIPI Camera ISP clocks and Camera Control Interface (CCI) buses managed exclusively by the OS kernel camera driver. |
| **WCN, PCIe CLKREQ & USB4 Sideband** | `WCN_*`, `CLKREQ_PCIE*`, `*_SBRX`, `*_SBTX`, `*_CC_DIR`, `*_SBU_*` | Controlled by the OS Wi-Fi/BT driver, SoC PCIe root complex driver, or USB4 retimer hardware sideband links. |

---

## 4. QUPv3 Serial Engine (`QUPV3_<x>_SE<y>`) Lane Matching Rule
On Qualcomm SoCs, a single GPIO pad can expose different QUP Serial Engines across `Alternate function 1` and `Alternate function 2` (for example, exposing `QUPx_SEa_L2/L3` in `Alternate function 1` and `QUPx_SEb_L0/L1` in `Alternate function 2`).
- Because **I2C always uses Lane 0 (SDA) and Lane 1 (SCL)** of a QUPv3 Serial Engine, the generator specifically matches `QUP<x>_SE<y>_L0` and `QUP<x>_SE<y>_L1` across `Alternate function 1` and `2` for I2C nets, while matching `L0..L3` in `Alternate function 1` for 4-wire SPI and UART buses.
