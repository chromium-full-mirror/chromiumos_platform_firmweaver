// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import (
	"context"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/base64"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"sync"
	"time"
)

var (
	tokenEndpoint    = DefaultTokenURL
	userInfoEndpoint = DefaultUserInfoURL
	browserLauncher  = openBrowser
)

type tokenResponse struct {
	AccessToken  string `json:"access_token"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int    `json:"expires_in"`
	RefreshToken string `json:"refresh_token"`
	Error        string `json:"error"`
	ErrorDesc    string `json:"error_description"`
}

type userInfoResponse struct {
	Email string `json:"email"`
}

// GetToken returns a valid OAuth access token and user email, refreshing if needed.
func GetToken(ctx context.Context) (string, string, error) {
	tok, err := LoadToken()
	if err != nil {
		return "", "", fmt.Errorf("reading token: %w", err)
	}
	if tok != nil {
		if tok.Valid() {
			return tok.AccessToken, tok.Email, nil
		}
		if tok.RefreshToken != "" {
			refreshed, refreshErr := RefreshToken(ctx, tok)
			if refreshErr == nil && refreshed.Valid() {
				return refreshed.AccessToken, refreshed.Email, nil
			}
			if refreshErr != nil {
				if strings.Contains(refreshErr.Error(), "invalid_grant") {
					_ = DeleteToken()
					return "", "", fmt.Errorf("session expired or revoked: run 'firmweaver auth login'")
				}
				return "", "", fmt.Errorf("failed to refresh authentication token: %w", refreshErr)
			}
		}
	}
	return "", "", fmt.Errorf("authentication required: run 'firmweaver auth login'")
}

// GetCachedIdentity returns the logged-in user's email if available.
func GetCachedIdentity() (string, bool) {
	tok, err := LoadToken()
	if err != nil || tok == nil || tok.Email == "" {
		return "", false
	}
	return tok.Email, true
}

// Login executes the PKCE OAuth 2.0 authorization code flow.
func Login(ctx context.Context) (*Token, error) {
	verifier, challenge, err := GeneratePKCE()
	if err != nil {
		return nil, err
	}
	state, err := GenerateState()
	if err != nil {
		return nil, err
	}

	// TODO: Running this over a remote SSH session will fail unless port 1123
	// is forwarded (e.g. ssh -L 1123:localhost:1123).
	listener, err := net.Listen("tcp", net.JoinHostPort(redirectHost, redirectPort))
	if err != nil {
		return nil, fmt.Errorf("failed to open local port %s: %w", redirectPort, err)
	}
	defer listener.Close()

	redirectURI := fmt.Sprintf("http://%s:%s%s", redirectHost, redirectPort, redirectPath)
	params := url.Values{
		"client_id":             {DefaultClientID},
		"redirect_uri":          {redirectURI},
		"response_type":         {"code"},
		"scope":                 {defaultScopes},
		"code_challenge":        {challenge},
		"code_challenge_method": {"S256"},
		"state":                 {state},
		"access_type":           {"offline"},
		"prompt":                {"consent"},
	}

	authURL := fmt.Sprintf("%s?%s", DefaultAuthURL, params.Encode())
	fmt.Fprintf(os.Stderr, "Open the following URL in your browser to authenticate:\n\n  %s\n\n", authURL)
	_ = browserLauncher(authURL)

	type callbackResult struct {
		code string
		err  error
	}
	resultChan := make(chan callbackResult, 1)
	var sendOnce sync.Once

	server := &http.Server{
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       10 * time.Second,
		WriteTimeout:      10 * time.Second,
		IdleTimeout:       15 * time.Second,
		Handler: http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			if r.URL.Path != redirectPath {
				http.NotFound(w, r)
				return
			}
			q := r.URL.Query()
			reqState := q.Get("state")
			if subtle.ConstantTimeCompare([]byte(reqState), []byte(state)) != 1 {
				http.Error(w, "Invalid state", http.StatusBadRequest)
				return
			}
			if authErr := q.Get("error"); authErr != "" {
				http.Error(w, authErr, http.StatusForbidden)
				sendOnce.Do(func() {
					resultChan <- callbackResult{err: fmt.Errorf("authorization denied: %s", authErr)}
				})
				return
			}
			code := q.Get("code")
			if code == "" {
				http.Error(w, "Missing code", http.StatusBadRequest)
				return
			}
			w.Header().Set("Content-Type", "text/plain; charset=utf-8")
			fmt.Fprintln(w, "Authentication successful! You can close this tab and return to the terminal.")
			sendOnce.Do(func() {
				resultChan <- callbackResult{code: code}
			})
		}),
	}

	shutdownServer := func() {
		shutdownCtx, cancel := context.WithTimeout(context.Background(), time.Second)
		defer cancel()
		if err := server.Shutdown(shutdownCtx); err != nil {
			_ = server.Close()
		}
	}
	defer shutdownServer()

	go func() { _ = server.Serve(listener) }()

	var code string
	timer := time.NewTimer(120 * time.Second)
	defer timer.Stop()

	select {
	case res := <-resultChan:
		if res.err != nil {
			return nil, res.err
		}
		code = res.code
	case <-timer.C:
		return nil, fmt.Errorf("authentication timed out")
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	tok, err := requestToken(ctx, url.Values{
		"client_id":     {DefaultClientID},
		"client_secret": {DefaultClientSecret},
		"grant_type":    {"authorization_code"},
		"code":          {code},
		"code_verifier": {verifier},
		"redirect_uri":  {redirectURI},
	})
	if err != nil {
		return nil, fmt.Errorf("token exchange failed: %w", err)
	}

	if email, err := fetchUserEmail(ctx, tok.AccessToken); err == nil {
		tok.Email = email
	}

	if err := SaveToken(tok); err != nil {
		return nil, fmt.Errorf("failed to save token: %w", err)
	}
	return tok, nil
}

// RefreshToken requests a new access token using the stored refresh token.
func RefreshToken(ctx context.Context, tok *Token) (*Token, error) {
	if tok == nil || tok.RefreshToken == "" {
		return nil, fmt.Errorf("no refresh token available")
	}

	refreshed, err := requestToken(ctx, url.Values{
		"client_id":     {DefaultClientID},
		"client_secret": {DefaultClientSecret},
		"grant_type":    {"refresh_token"},
		"refresh_token": {tok.RefreshToken},
	})
	if err != nil {
		return nil, fmt.Errorf("token refresh failed: %w", err)
	}

	tok.AccessToken = refreshed.AccessToken
	tok.Expiry = refreshed.Expiry
	if refreshed.RefreshToken != "" {
		tok.RefreshToken = refreshed.RefreshToken
	}

	if err := SaveToken(tok); err != nil {
		return nil, fmt.Errorf("failed to save token: %w", err)
	}
	return tok, nil
}

func requestToken(ctx context.Context, data url.Values) (*Token, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, tokenEndpoint, strings.NewReader(data.Encode()))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	resp, err := (&http.Client{Timeout: 10 * time.Second}).Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body, err := io.ReadAll(io.LimitReader(resp.Body, 1<<20))
	if err != nil {
		return nil, fmt.Errorf("reading token response: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		var errRes tokenResponse
		if jsonErr := json.Unmarshal(body, &errRes); jsonErr == nil && errRes.Error != "" {
			return nil, fmt.Errorf("%s: %s", errRes.Error, errRes.ErrorDesc)
		}
		return nil, fmt.Errorf("token server returned status %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	var res tokenResponse
	if err := json.Unmarshal(body, &res); err != nil {
		return nil, fmt.Errorf("failed to decode token response: %w", err)
	}

	expiresIn := time.Duration(res.ExpiresIn) * time.Second
	if expiresIn <= 0 {
		expiresIn = defaultTokenExpiry
	}

	return &Token{
		AccessToken:  res.AccessToken,
		TokenType:    res.TokenType,
		RefreshToken: res.RefreshToken,
		Expiry:       time.Now().Add(expiresIn),
	}, nil
}

func fetchUserEmail(ctx context.Context, accessToken string) (string, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, userInfoEndpoint, nil)
	if err != nil {
		return "", err
	}
	req.Header.Set("Authorization", "Bearer "+accessToken)

	resp, err := (&http.Client{Timeout: 5 * time.Second}).Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("userinfo request failed with status %d", resp.StatusCode)
	}

	var res userInfoResponse
	if err := json.NewDecoder(resp.Body).Decode(&res); err != nil {
		return "", err
	}
	return res.Email, nil
}

// GeneratePKCE creates a secure random verifier and S256 challenge.
func GeneratePKCE() (verifier, challenge string, err error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", "", err
	}
	verifier = base64.RawURLEncoding.EncodeToString(b)
	h := sha256.Sum256([]byte(verifier))
	return verifier, base64.RawURLEncoding.EncodeToString(h[:]), nil
}

// GenerateState creates a random hex CSRF state parameter.
func GenerateState() (string, error) {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}

func openBrowser(targetURL string) error {
	switch runtime.GOOS {
	case "linux":
		return exec.Command("xdg-open", targetURL).Start()
	case "darwin":
		return exec.Command("open", targetURL).Start()
	case "windows":
		return exec.Command("rundll32", "url.dll,FileProtocolHandler", targetURL).Start()
	default:
		return fmt.Errorf("unsupported platform")
	}
}
