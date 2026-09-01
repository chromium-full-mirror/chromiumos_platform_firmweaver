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
    *   AP GPIO Configuration (`coreboot_gpio`, alias: `gpio`)
    *   EC Devicetree (`zephyr_dtsi`, alias: `dtsi`)

## Getting Started

### Building Firmweaver

Build the `firmweaver` binary from the repository root:

```bash
go build -o firmweaver .
```

## Usage

### Running the Coreboot GPIO Tool

The `coreboot_gpio` (alias: `gpio`) parses a GPIO map CSV file and generates C configuration code.

```bash
./firmweaver coreboot_gpio --soc <soc_type> --csv <path_to_csv> [--output <path_to_output_c_file>]
```

#### Arguments:
*   `--soc`: SoC type. Supported values: `intel`, `qcom`, `mtk`. (Required)
*   `--csv`: Path to the local CSV file containing the GPIO map. (Required)
*   `--output`: Path to the output file. If omitted, output is written to stdout.

#### Example:
To generate Intel GPIO configuration from the provided example CSV:
```bash
./firmweaver coreboot_gpio --soc intel --csv coreboot_gpio/test/ExampleGpio.csv --output coreboot_gpio/test/gpio_config.c
```
*(You can also use the shorthand `./firmweaver gpio ...`)*

### Running the EC Zephyr Devicetree Generator

The `zephyr_dtsi` (alias: `dtsi`) parses an EC pinout CSV spreadsheet and generates Zephyr devicetree overlays.

```bash
./firmweaver zephyr_dtsi --chip <chip_name> --csv <path_to_csv> [--pin-col <variant>] [--output <path_to_output_dtsi>]
```

#### Example:
To generate Nuvoton NPCX9 Devicetree overlay from the provided example CSV:
```bash
./firmweaver zephyr_dtsi --chip npcx99 --csv zephyr_dtsi/test/ExampleEcPinout.csv --pin-col "NPCX99 (NPCX)" --output zephyr_dtsi/test/generated.dtsi
```
*(You can also use the shorthand `./firmweaver dtsi ...`)*

## Testing

Run unit tests across all tools in the workspace:

```bash
go test -v ./...
```

## Contributing

This project follows the standard Chromium OS contribution process.
