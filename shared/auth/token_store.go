// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// TokenFilePath returns the path to the cached token file.
func TokenFilePath() string {
	if env := os.Getenv(ConfigDirEnv); env != "" {
		return filepath.Join(env, TokenFileName)
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		dir := filepath.Join(os.TempDir(), fmt.Sprintf("firmweaver-%d", os.Getuid()))
		return filepath.Join(dir, TokenFileName)
	}
	return filepath.Join(home, ".config", "firmweaver", TokenFileName)
}

// LoadToken reads and unmarshals the cached token from disk.
func LoadToken() (*Token, error) {
	data, err := os.ReadFile(TokenFilePath())
	if err != nil {
		if os.IsNotExist(err) {
			return nil, nil
		}
		return nil, fmt.Errorf("failed to read token: %w", err)
	}

	var tok Token
	if err := json.Unmarshal(data, &tok); err != nil {
		return nil, fmt.Errorf("corrupted token file: %w", err)
	}
	return &tok, nil
}

// SaveToken writes the token to disk atomically with 0600 permissions.
func SaveToken(tok *Token) error {
	if tok == nil {
		return fmt.Errorf("cannot save nil token")
	}

	path := TokenFilePath()
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0700); err != nil {
		return fmt.Errorf("failed to create config directory: %w", err)
	}
	_ = os.Chmod(dir, 0700)

	data, err := json.MarshalIndent(tok, "", "  ")
	if err != nil {
		return fmt.Errorf("failed to marshal token: %w", err)
	}

	tmpFile, err := os.CreateTemp(dir, ".tmp_token_*")
	if err != nil {
		return fmt.Errorf("creating temp token file: %w", err)
	}
	tmpPath := tmpFile.Name()
	defer func() {
		if tmpFile != nil {
			_ = tmpFile.Close()
			_ = os.Remove(tmpPath)
		}
	}()

	if err := tmpFile.Chmod(0600); err != nil {
		return fmt.Errorf("setting temp token permissions: %w", err)
	}
	if _, err := tmpFile.Write(data); err != nil {
		return fmt.Errorf("writing temp token file: %w", err)
	}
	if err := tmpFile.Close(); err != nil {
		return fmt.Errorf("closing temp token file: %w", err)
	}
	tmpFile = nil

	if err := os.Rename(tmpPath, path); err != nil {
		_ = os.Remove(tmpPath)
		return fmt.Errorf("renaming token file: %w", err)
	}
	return nil
}

// DeleteToken removes the token file from disk.
func DeleteToken() error {
	if err := os.Remove(TokenFilePath()); err != nil && !os.IsNotExist(err) {
		return fmt.Errorf("failed to delete token: %w", err)
	}
	return nil
}
