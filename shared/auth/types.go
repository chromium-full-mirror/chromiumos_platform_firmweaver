// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package auth

import "time"

const (
	ConfigDirEnv  = "FIRMWEAVER_CONFIG_DIR"
	TokenFileName = "token.json"

	DefaultClientID     = "151040046340-8krotv9fbrdpe81f2be03dk789jdnl6p.apps.googleusercontent.com"
	DefaultClientSecret = "LMJASl09TJQBXa6Ia1JL9Xea"

	DefaultAuthURL     = "https://accounts.google.com/o/oauth2/v2/auth"
	DefaultTokenURL    = "https://oauth2.googleapis.com/token"
	DefaultUserInfoURL = "https://www.googleapis.com/oauth2/v2/userinfo"

	defaultScopes = "https://www.googleapis.com/auth/drive.readonly https://www.googleapis.com/auth/bigquery.insertdata https://www.googleapis.com/auth/userinfo.email openid"

	redirectHost = "127.0.0.1"
	// Port 1123 and path /arb-auth match the pre-registered OAuth callback URL
	// for this Client ID in Google Cloud Console. Changing them will cause
	// Google to reject authentication with a redirect URI mismatch error.
	redirectPort = "1123"
	redirectPath = "/arb-auth"

	defaultTokenExpiry = 3600 * time.Second
)

// Token represents a cached OAuth2 token set.
type Token struct {
	AccessToken  string    `json:"access_token"`
	TokenType    string    `json:"token_type,omitempty"`
	RefreshToken string    `json:"refresh_token,omitempty"`
	Expiry       time.Time `json:"expiry"`
	Email        string    `json:"email,omitempty"`
}

// Valid returns true if the access token exists and has at least 2 minutes of validity remaining.
func (t *Token) Valid() bool {
	if t == nil || t.AccessToken == "" {
		return false
	}
	return time.Now().Add(2 * time.Minute).Before(t.Expiry)
}
