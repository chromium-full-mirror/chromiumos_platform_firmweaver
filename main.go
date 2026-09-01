// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"os"

	"firmweaver/coreboot_gpio"
	"firmweaver/shared/auth"
	"firmweaver/zephyr_dtsi"
)

func printUsage(w io.Writer) {
	fmt.Fprintln(w, `Firmweaver

Usage:
  firmweaver <command> [flags...]

Available Commands:
  auth                 Manage Google authentication (login, status, logout, token)
  coreboot_gpio (alias: gpio)    Generate AP SoC GPIO configurations
  zephyr_dtsi   (alias: dtsi)    Generate EC Devicetree (DTSI) pinmux configurations`)
}

func main() {
	if err := run(os.Args[1:]); err != nil {
		if errors.Is(err, flag.ErrHelp) {
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

func run(args []string) error {
	if len(args) == 0 {
		printUsage(os.Stderr)
		return fmt.Errorf("subcommand required: 'auth', 'coreboot_gpio', or 'zephyr_dtsi'")
	}

	switch args[0] {
	case "auth":
		return auth.HandleAuthCLI(context.Background(), args[1:])
	case "coreboot_gpio", "gpio":
		err := coreboot_gpio.Run(args[1:])
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	case "zephyr_dtsi", "dtsi":
		err := zephyr_dtsi.Run(args[1:])
		if errors.Is(err, flag.ErrHelp) {
			return nil
		}
		return err
	case "-h", "--help", "help":
		printUsage(os.Stdout)
		return nil
	default:
		printUsage(os.Stderr)
		return fmt.Errorf("unknown command %q", args[0])
	}
}
