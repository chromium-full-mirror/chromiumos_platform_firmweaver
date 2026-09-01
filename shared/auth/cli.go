// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"context"
	"fmt"
)

// HandleAuthCLI handles the `auth` subcommand (login, status, logout, token).
func HandleAuthCLI(ctx context.Context, args []string) error {
	action := "status"
	if len(args) > 0 {
		action = args[0]
	}
	switch action {
	case "login":
		tok, err := Login(ctx)
		if err != nil {
			return fmt.Errorf("login failed: %w", err)
		}
		if tok.Email != "" {
			fmt.Printf("Logged in as: %s\n", tok.Email)
		} else {
			fmt.Println("Logged in.")
		}
		return nil
	case "status":
		tok, err := LoadToken()
		if err != nil {
			return fmt.Errorf("reading token: %w", err)
		}
		if tok == nil || (!tok.Valid() && tok.RefreshToken == "") {
			fmt.Println("Not logged in. Run 'firmweaver auth login' to authenticate.")
			return nil
		}
		if tok.Email != "" {
			fmt.Printf("Logged in as: %s\n", tok.Email)
		} else {
			fmt.Println("Logged in.")
		}
		return nil
	case "token":
		tokStr, _, err := GetToken(ctx)
		if err != nil {
			return fmt.Errorf("failed to retrieve token: %w", err)
		}
		fmt.Println(tokStr)
		return nil
	case "logout":
		if err := DeleteToken(); err != nil {
			return err
		}
		fmt.Println("Logged out.")
		return nil
	default:
		return fmt.Errorf("unknown auth command %q. Usage: firmweaver auth [login|status|logout|token]", action)
	}
}
