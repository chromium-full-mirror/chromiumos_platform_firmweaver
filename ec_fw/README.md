# EC Firmware Devicetree Generator (`ec_fw`)

`ec_fw` is a tool for generating Zephyr EC Devicetree configuration files (`generated.dtsi`) for Chromium OS EC board bringup.

It ingests hardware pinout CSV spreadsheets prepared by board design teams and outputs a self-contained Zephyr Devicetree overlay containing named GPIOs, ADC channels, PWM channels, I2C ports, TACH channels, and Keyboard matrix configurations.

---

## Features

- **Variant Pinout Filtering (`--pin-col`)**: Selects variant-specific pin mapping columns directly from multi-variant board spreadsheets (e.g., `--pin-col="NPCX99 (NPCX)"` or `--pin-col="IT82002 (ITE)"`).
- **Pin Maps**: Built-in chip pin maps are stored as embedded JSON data files (`data/chips/*.json`) and can be extended dynamically at runtime using `--chip-data-dir`.

---

## Supported EC Chips

| Chip Family | Canonical Name | Supported Aliases / Variants | Pin Map Data File |
|---|---|---|---|
| **Nuvoton NPCX9** | `npcx99` | `npcx993`, `npcx996`, `npcx9` | [data/chips/npcx99.json](data/chips/npcx99.json) |
| **ITE IT81302** | `it81302` | `it81302` | [data/chips/it81302.json](data/chips/it81302.json) |
| **ITE IT81202** | `it81202` | `it81202` | [data/chips/it81202.json](data/chips/it81202.json) |
| **ITE IT82002** | `it82002` | `it82002` | [data/chips/it82002.json](data/chips/it82002.json) |
| **ITE IT82202** | `it82202` | `it82202ax` | [data/chips/it82202.json](data/chips/it82202.json) |

---

## CLI Reference

```bash
go run ./ec_fw --chip=<chip_name> --csv=<path_to_csv> [options]
```

### Options

| Flag | Required | Description | Default |
|---|---|---|---|
| `--chip` | **Yes** | Target EC chip model or variant alias (e.g. `npcx993`, `it81302`, `it82202ax`). | `""` |
| `--csv` | **Yes** | Path to the local CSV file containing the EC pinout table. | `""` |
| `--pin-col` | No | Variant column header name in multi-variant spreadsheets (e.g. `"NPCX99 (NPCX)"`, `"IT82002 (ITE)"`). | `""` (defaults to `PinNumber` / `Pin`) |
| `--chip-data-dir` | No | Path to an external directory containing custom chip JSON pin maps. | `""` |
| `--output` | No | Path to write the output `generated.dtsi` file. | `stdout` |

---

## Usage Examples

### 1. Generating Devicetree Using the Bundled Test Example
To test generating a `generated.dtsi` file from the example test CSV included in `ec_fw/test/`:

For NPCX (using `npcx99` chip):
```bash
go run ./ec_fw --chip=npcx99 \
               --csv=ec_fw/test/ExampleEcPinout.csv \
               --pin-col="NPCX99 (NPCX)" \
               --output=ec_fw/test/generated_npcx.dtsi
```

For ITE (using `it82002` chip):
```bash
go run ./ec_fw --chip=it82002 \
               --csv=ec_fw/test/ExampleEcPinout.csv \
               --pin-col="IT82002 (ITE)" \
               --output=ec_fw/test/generated_ite.dtsi
```

### 2. Generating Devicetree for Board Variants
To generate a `generated.dtsi` file for the Fatcat **RVP** (NPCX chip):

```bash
go run ./ec_fw --chip=npcx99 \
               --csv=/path/to/fatcat.csv \
               --pin-col="NPCx9 RVP" \
               --output=/path/to/platform/ec/zephyr/program/fatcat/generated_npcx.dtsi
```

To generate for the Fatcat **ITE** variant:

```bash
go run ./ec_fw --chip=it82002 \
               --csv=/path/to/fatcat.csv \
               --pin-col="IT82002 Ref" \
               --output=/path/to/platform/ec/zephyr/program/fatcat/generated_ite.dtsi
```

### 3. Building and Running Binary
Build the standalone `ec_fw` binary from the repository root:

```bash
go build -o ec_fw ./ec_fw
./ec_fw --chip=it82202ax --csv=/path/to/hylia.csv --output=generated.dtsi
```

### 4. Adding Custom Chip Pin Maps
To register custom EC chip models at runtime without modifying code:
1. Create a JSON file (e.g. `custom_chip.json`):
   ```json
   {
     "name": "custom_chip",
     "aliases": ["custom_v1"],
     "pin_map": {
       "A1": "GPIOB4/I2C0_SDA0",
       "A2": "GPIOB5/I2C0_SCL0"
     }
   }
   ```
2. Pass the directory containing your JSON file to `--chip-data-dir`:
   ```bash
   go run ./ec_fw --chip-data-dir=/path/to/custom_chips --chip=custom_chip --csv=board.csv
   ```

### 5. Running Tests
Run unit tests across the `ec_fw` tool:

```bash
go test -v ./ec_fw/...
```
