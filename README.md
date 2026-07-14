# Firmweaver

Firmweaver is a collection of tools for Chromium OS to assist with initial
firmware setup during board bringup.

## Introduction

The Chromium OS project includes tools to assist with initial firmware setup
during board bringup. These tools generate certain objects in Application
Processor (AP) and Embedded Controller (EC) firmware (e.g., EC devicetree,
AP GPIO configuration, etc.).

The tools take a curated sheet as prepared by the hardware team as an input and
generate these objects as output. Developers can validate the generated output
and contribute them to the concerned project.

## Features

*   **Automatic Generation**: Generates firmware objects from hardware sheets.
*   **Supported Outputs**:
    *   EC Devicetree (`ec_fw`)
    *   AP GPIO Configuration (`ap_fw`)

## Getting Started

### Building the GPIO Tool

The GPIO tool is located in the `ap_fw` directory.

1. Navigate to the `ap_fw` directory:
   ```bash
   cd ap_fw
   ```
2. Build the tool:
   ```bash
   go build -o gpio_tool
   ```
   This will generate the `gpio_tool` executable in the `ap_fw` directory.

### Building the EC Devicetree Generator

The EC Devicetree generator is located in the `ec_fw` directory. See [`ec_fw/README.md`](ec_fw/README.md) for full details.

1. Navigate to the `ec_fw` directory:
   ```bash
   cd ec_fw
   ```
2. Build the tool:
   ```bash
   go build -o ec_fw
   ```
   This will generate the `ec_fw` executable in the `ec_fw` directory.

## Usage

### Running the GPIO Tool

The `gpio_tool` parses a GPIO map CSV file and generates C configuration code.

```bash
./ap_fw/gpio_tool --soc <soc_type> --csv <path_to_csv> [--output <path_to_output_c_file>]
```

#### Arguments:
*   `--soc`: SoC type. Supported values: `intel`, `qcom`, `mtk`. (Required)
*   `--csv`: Path to the local CSV file containing the GPIO map. (Required)
*   `--output`: Path to the output file. If omitted, output is written to stdout.

#### Example:
To generate Intel GPIO configuration from the provided example CSV:
```bash
./ap_fw/gpio_tool --soc intel --csv ap_fw/test/ExampleGpio.csv --output ap_fw/test/gpio_config.c
```

### Running the EC Devicetree Generator

The `ec_fw` tool parses an EC pinout CSV spreadsheet and generates Zephyr devicetree overlays.

```bash
./ec_fw/ec_fw --chip <chip_name> --csv <path_to_csv> [--pin-col <variant>] [--output <path_to_output_dtsi>]
```

#### Example:
To generate Nuvoton NPCX9 Devicetree overlay from the provided example CSV:
```bash
./ec_fw/ec_fw --chip npcx99 --csv ec_fw/test/ExampleEcPinout.csv --pin-col "NPCX99 (NPCX)" --output ec_fw/test/generated.dtsi
```

## Testing

Run unit tests across all tools in the workspace:

```bash
go test -v ./...
```

## Contributing

This project follows the standard Chromium OS contribution process.
