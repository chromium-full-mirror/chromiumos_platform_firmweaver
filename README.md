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
    *   EC Devicetree
    *   AP GPIO Configuration

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

## Contributing

This project follows the standard Chromium OS contribution process.
