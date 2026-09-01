// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"runtime"
	"strings"
	"testing"
	"time"
)

const testUserEmail = "developer@google.com"

func TestPKCEGeneration(t *testing.T) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		t.Fatalf("GeneratePKCE() failed: %v", err)
	}
	if len(verifier) != 43 {
		t.Errorf("expected verifier length 43, got %d", len(verifier))
	}
	if len(challenge) != 43 {
		t.Errorf("expected challenge length 43, got %d", len(challenge))
	}

	state, err := GenerateState()
	if err != nil {
		t.Fatalf("GenerateState() failed: %v", err)
	}
	if len(state) != 32 {
		t.Errorf("expected state length 32, got %d", len(state))
	}
}

func TestTokenValidity(t *testing.T) {
	var nilTok *Token
	if nilTok.Valid() {
		t.Errorf("nil token should not be valid")
	}

	emptyTok := &Token{}
	if emptyTok.Valid() {
		t.Errorf("empty token should not be valid")
	}

	expiredTok := &Token{
		AccessToken: "mock-expired",
		Expiry:      time.Now().Add(-10 * time.Minute),
	}
	if expiredTok.Valid() {
		t.Errorf("expired token should not be valid")
	}

	validTok := &Token{
		AccessToken: "mock-valid",
		Expiry:      time.Now().Add(30 * time.Minute),
	}
	if !validTok.Valid() {
		t.Errorf("token expiring in 30m should be valid")
	}
}

func TestTokenStorePersistence(t *testing.T) {
	tempDir := t.TempDir()
	t.Setenv(ConfigDirEnv, tempDir)

	tok := &Token{
		AccessToken:  "test-access-token",
		RefreshToken: "test-refresh-token",
		Expiry:       time.Now().Add(time.Hour).Truncate(time.Second),
		Email:        testUserEmail,
	}

	if err := SaveToken(tok); err != nil {
		t.Fatalf("SaveToken() failed: %v", err)
	}

	// Verify file permissions 0600
	info, err := os.Stat(TokenFilePath())
	if err != nil {
		t.Fatalf("Stat token file failed: %v", err)
	}
	if runtime.GOOS != "windows" {
		if perm := info.Mode().Perm(); perm != 0600 {
			t.Errorf("expected token file mode 0600, got %o", perm)
		}
	}

	loaded, err := LoadToken()
	if err != nil {
		t.Fatalf("LoadToken() failed: %v", err)
	}
	if loaded.AccessToken != tok.AccessToken || loaded.RefreshToken != tok.RefreshToken || loaded.Email != tok.Email {
		t.Errorf("loaded token mismatch: got %+v, want %+v", loaded, tok)
	}

	email, ok := GetCachedIdentity()
	if !ok || email != testUserEmail {
		t.Errorf("GetCachedIdentity() = (%s, %v), want (%s, true)", email, ok, testUserEmail)
	}

	if err := DeleteToken(); err != nil {
		t.Fatalf("DeleteToken() failed: %v", err)
	}
	if loadedAfterDelete, _ := LoadToken(); loadedAfterDelete != nil {
		t.Errorf("LoadToken() after delete should be nil, got %+v", loadedAfterDelete)
	}
}

func TestRefreshTokenFlow(t *testing.T) {
	t.Setenv(ConfigDirEnv, t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken: "refreshed-access-token",
			TokenType:   "Bearer",
			ExpiresIn:   3600,
		})
	}))
	defer server.Close()

	orig := tokenEndpoint
	tokenEndpoint = server.URL
	t.Cleanup(func() { tokenEndpoint = orig })

	tok := &Token{
		AccessToken:  "old-expired-token",
		RefreshToken: "valid-refresh-token",
		Expiry:       time.Now().Add(-time.Hour),
		Email:        testUserEmail,
	}
	_ = SaveToken(tok)

	tokenStr, email, err := GetToken(context.Background())
	if err != nil {
		t.Fatalf("GetToken() failed: %v", err)
	}
	if tokenStr != "refreshed-access-token" || email != testUserEmail {
		t.Errorf("GetToken() = (%s, %s), want (refreshed-access-token, %s)", tokenStr, email, testUserEmail)
	}
}

func TestRefreshToken_OfflineError(t *testing.T) {
	t.Setenv(ConfigDirEnv, t.TempDir())

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		http.Error(w, "server down", http.StatusBadGateway)
	}))
	server.Close()

	orig := tokenEndpoint
	tokenEndpoint = server.URL
	t.Cleanup(func() { tokenEndpoint = orig })

	tok := &Token{
		AccessToken:  "old-expired-token",
		RefreshToken: "valid-refresh-token",
		Expiry:       time.Now().Add(-time.Hour),
		Email:        testUserEmail,
	}
	_ = SaveToken(tok)

	_, _, err := GetToken(context.Background())
	if err == nil || !strings.Contains(err.Error(), "failed to refresh authentication token") {
		t.Errorf("expected refresh network error, got: %v", err)
	}
}

func TestFetchUserEmail(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Header.Get("Authorization") != "Bearer valid-token" {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		_ = json.NewEncoder(w).Encode(userInfoResponse{Email: testUserEmail})
	}))
	defer server.Close()

	orig := userInfoEndpoint
	userInfoEndpoint = server.URL
	t.Cleanup(func() { userInfoEndpoint = orig })

	email, err := fetchUserEmail(context.Background(), "valid-token")
	if err != nil || email != testUserEmail {
		t.Fatalf("fetchUserEmail() = %s, err: %v", email, err)
	}
}

func TestHandleAuthCLI(t *testing.T) {
	t.Setenv(ConfigDirEnv, t.TempDir())
	ctx := context.Background()

	if err := HandleAuthCLI(ctx, []string{"status"}); err != nil {
		t.Errorf("status failed: %v", err)
	}
	if err := HandleAuthCLI(ctx, []string{"token"}); err == nil {
		t.Errorf("token when logged out should return error")
	}

	_ = SaveToken(&Token{
		AccessToken: "cli-token",
		Expiry:      time.Now().Add(time.Hour),
		Email:       testUserEmail,
	})

	if err := HandleAuthCLI(ctx, []string{"status"}); err != nil {
		t.Errorf("status failed: %v", err)
	}
	if err := HandleAuthCLI(ctx, []string{"token"}); err != nil {
		t.Errorf("token failed: %v", err)
	}
	if err := HandleAuthCLI(ctx, []string{"logout"}); err != nil {
		t.Errorf("logout failed: %v", err)
	}
	if err := HandleAuthCLI(ctx, []string{"invalid_cmd"}); err == nil {
		t.Errorf("invalid command should return error")
	}
}

func TestLogin_FullFlow(t *testing.T) {
	t.Setenv(ConfigDirEnv, t.TempDir())

	tokenServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(tokenResponse{
			AccessToken:  "mock-code-token",
			TokenType:    "Bearer",
			ExpiresIn:    3600,
			RefreshToken: "mock-refresh-token",
		})
	}))
	defer tokenServer.Close()

	userServer := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(userInfoResponse{Email: testUserEmail})
	}))
	defer userServer.Close()

	oldToken := tokenEndpoint
	oldUser := userInfoEndpoint
	oldLauncher := browserLauncher
	tokenEndpoint = tokenServer.URL
	userInfoEndpoint = userServer.URL
	t.Cleanup(func() {
		tokenEndpoint = oldToken
		userInfoEndpoint = oldUser
		browserLauncher = oldLauncher
	})

	browserLauncher = func(targetURL string) error {
		u, err := url.Parse(targetURL)
		if err != nil {
			return err
		}
		state := u.Query().Get("state")
		go func() {
			time.Sleep(30 * time.Millisecond)
			// Stray probe with invalid state (should be ignored by callback server)
			probeURL := fmt.Sprintf("http://%s:%s%s?state=wrong_state&code=probe", redirectHost, redirectPort, redirectPath)
			if resp, err := http.Get(probeURL); err == nil {
				_ = resp.Body.Close()
			}

			// Valid callback
			callbackURL := fmt.Sprintf("http://%s:%s%s?state=%s&code=test-auth-code", redirectHost, redirectPort, redirectPath, state)
			resp, err := http.Get(callbackURL)
			if err == nil {
				_ = resp.Body.Close()
			}
		}()
		return nil
	}

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()

	tok, err := Login(ctx)
	if err != nil {
		t.Fatalf("Login() failed: %v", err)
	}
	if tok.AccessToken != "mock-code-token" || tok.Email != testUserEmail {
		t.Errorf("unexpected token from Login(): %+v", tok)
	}
}
