# EC Firmware Devicetree Generator (`zephyr_dtsi`)

`zephyr_dtsi` is a tool for generating Zephyr EC Devicetree configuration files (`generated.dtsi`) for Chromium OS EC board bringup.

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
./firmweaver dtsi --chip=<chip_name> --csv=<path_to_csv> [options]
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
To test generating a `generated.dtsi` file from the example test CSV included in `zephyr_dtsi/test/`:

For NPCX (using `npcx99` chip):
```bash
./firmweaver dtsi --chip=npcx99 \
               --csv=zephyr_dtsi/test/ExampleEcPinout.csv \
               --pin-col="NPCX99 (NPCX)" \
               --output=zephyr_dtsi/test/generated_npcx.dtsi
```

For ITE (using `it82002` chip):
```bash
./firmweaver dtsi --chip=it82002 \
               --csv=zephyr_dtsi/test/ExampleEcPinout.csv \
               --pin-col="IT82002 (ITE)" \
               --output=zephyr_dtsi/test/generated_ite.dtsi
```

### 2. Generating Devicetree for Board Variants
To generate a `generated.dtsi` file for the Fatcat **RVP** (NPCX chip):

```bash
./firmweaver dtsi --chip=npcx99 \
               --csv=/path/to/fatcat.csv \
               --pin-col="NPCx9 RVP" \
               --output=/path/to/platform/ec/zephyr/program/fatcat/generated_npcx.dtsi
```

To generate for the Fatcat **ITE** variant:

```bash
./firmweaver dtsi --chip=it82002 \
               --csv=/path/to/fatcat.csv \
               --pin-col="IT82002 Ref" \
               --output=/path/to/platform/ec/zephyr/program/fatcat/generated_ite.dtsi
```

### 3. Adding Custom Chip Pin Maps
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
   ./firmweaver dtsi --chip-data-dir=/path/to/custom_chips --chip=custom_chip --csv=board.csv
   ```

### 4. Running Tests
Run unit tests across the `zephyr_dtsi` tool:

```bash
go test -v ./zephyr_dtsi/...
```
