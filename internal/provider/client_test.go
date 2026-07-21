// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNewClient_NormalizesURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name string
		in   string
		want string
	}{
		{"trailing slash stripped", "https://netbox.example.com/", "https://netbox.example.com"},
		{"trailing /api stripped", "https://netbox.example.com/api", "https://netbox.example.com"},
		{"trailing /api/ stripped", "https://netbox.example.com/api/", "https://netbox.example.com"},
		{"subpath preserved", "https://example.com/netbox", "https://example.com/netbox"},
		{"subpath with /api stripped", "https://example.com/netbox/api/", "https://example.com/netbox"},
		{"http scheme accepted", "http://localhost:8000", "http://localhost:8000"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			c, err := NewClient(tc.in, "token", false)
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if c.BaseURL != tc.want {
				t.Errorf("BaseURL = %q, want %q", c.BaseURL, tc.want)
			}
		})
	}
}

func TestNewClient_Errors(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name    string
		rawURL  string
		token   string
		wantSub string
	}{
		{"empty url", "", "t", "URL must not be empty"},
		{"empty token", "https://netbox.example.com", "", "token must not be empty"},
		{"bad scheme", "ftp://netbox.example.com", "t", "http or https"},
		{"no host", "https://", "t", "missing a host"},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := NewClient(tc.rawURL, tc.token, false)
			if err == nil {
				t.Fatal("expected error, got nil")
			}
			if !strings.Contains(err.Error(), tc.wantSub) {
				t.Errorf("error %q does not contain %q", err.Error(), tc.wantSub)
			}
		})
	}
}

func TestClient_Ping_SendsTokenAndSucceedsOn200(t *testing.T) {
	t.Parallel()

	var gotAuth string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/status/" {
			t.Errorf("unexpected path %q", r.URL.Path)
		}
		gotAuth = r.Header.Get("Authorization")
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"netbox-version":"4.0.0"}`))
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(srv.URL, "abc123", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	if err := c.Ping(context.Background()); err != nil {
		t.Fatalf("Ping: %v", err)
	}
	if gotAuth != "Token abc123" {
		t.Errorf("Authorization header = %q, want %q", gotAuth, "Token abc123")
	}
}

func TestClient_Ping_Fails401(t *testing.T) {
	t.Parallel()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusUnauthorized)
	}))
	t.Cleanup(srv.Close)

	c, err := NewClient(srv.URL, "abc123", false)
	if err != nil {
		t.Fatalf("NewClient: %v", err)
	}

	err = c.Ping(context.Background())
	if err == nil || !strings.Contains(err.Error(), "rejected the API token") {
		t.Fatalf("expected token-rejected error, got %v", err)
	}
}

func TestParseBoolEnv(t *testing.T) {
	t.Parallel()

	truthy := []string{"1", "t", "T", "true", "TRUE", "yes", "YES", "on", "ON"}
	falsy := []string{"", "0", "false", "no", "off", "garbage"}

	for _, v := range truthy {
		if !parseBoolEnv(v) {
			t.Errorf("parseBoolEnv(%q) = false, want true", v)
		}
	}
	for _, v := range falsy {
		if parseBoolEnv(v) {
			t.Errorf("parseBoolEnv(%q) = true, want false", v)
		}
	}
}
