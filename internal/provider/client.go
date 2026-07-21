// Copyright IBM Corp. 2021, 2025
// SPDX-License-Identifier: MPL-2.0

package provider

import (
	"bytes"
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// ErrNotFound is returned by Client JSON helpers when the NetBox API
// responds with HTTP 404. Callers can use errors.Is to detect a missing
// resource without inspecting HTTP-level details.
var ErrNotFound = errors.New("netbox: resource not found")

// APIError represents a non-2xx HTTP response from the NetBox API. The
// raw response body is preserved verbatim in Body for callers that want
// to surface it in diagnostics.
type APIError struct {
	StatusCode int
	Status     string
	Method     string
	Path       string
	Body       string
}

func (e *APIError) Error() string {
	body := strings.TrimSpace(e.Body)
	if body == "" {
		return fmt.Sprintf("netbox API %s %s returned %s", e.Method, e.Path, e.Status)
	}
	return fmt.Sprintf("netbox API %s %s returned %s: %s", e.Method, e.Path, e.Status, body)
}

// Client is a thin HTTP client for the NetBox REST API. It carries the
// base URL of a NetBox instance and the API token used to authenticate
// requests. Resources and data sources receive a *Client from the
// provider's Configure method via ResourceData / DataSourceData.
type Client struct {
	// BaseURL is the NetBox server URL, without a trailing slash and
	// without the "/api" suffix (for example: "https://netbox.example.com").
	BaseURL string

	// Token is the NetBox API token used for authentication.
	Token string

	// HTTPClient is the underlying HTTP client used to make requests.
	HTTPClient *http.Client
}

// NewClient constructs a Client. rawURL may include or omit a trailing
// slash; any trailing slash and any "/api" suffix are stripped so the
// stored BaseURL is a canonical origin (scheme + host [+ port]).
// If insecure is true, TLS certificate verification is disabled.
func NewClient(rawURL, token string, insecure bool) (*Client, error) {
	if rawURL == "" {
		return nil, fmt.Errorf("netbox URL must not be empty")
	}
	if token == "" {
		return nil, fmt.Errorf("netbox API token must not be empty")
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return nil, fmt.Errorf("invalid netbox URL %q: %w", rawURL, err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return nil, fmt.Errorf("netbox URL must use http or https scheme, got %q", parsed.Scheme)
	}
	if parsed.Host == "" {
		return nil, fmt.Errorf("netbox URL %q is missing a host", rawURL)
	}

	// Normalize: strip any path, then re-attach any leading path segments
	// the user provided EXCEPT a trailing "/api" or trailing slash.
	path := strings.TrimRight(parsed.Path, "/")
	path = strings.TrimSuffix(path, "/api")
	parsed.Path = path
	parsed.RawQuery = ""
	parsed.Fragment = ""

	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			InsecureSkipVerify: insecure, //nolint:gosec // user opt-in via provider config
			MinVersion:         tls.VersionTLS12,
		},
	}

	return &Client{
		BaseURL: strings.TrimRight(parsed.String(), "/"),
		Token:   token,
		HTTPClient: &http.Client{
			Timeout:   30 * time.Second,
			Transport: transport,
		},
	}, nil
}

// NewRequest builds an *http.Request against the NetBox API, setting the
// Authorization, Accept and Content-Type headers. path should start with
// "/api/..." or a path beginning with "/"; a leading slash is enforced.
func (c *Client) NewRequest(ctx context.Context, method, path string, body io.Reader) (*http.Request, error) {
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}
	req, err := http.NewRequestWithContext(ctx, method, c.BaseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Authorization", "Token "+c.Token)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	return req, nil
}

// Ping verifies that the provided URL and token can reach the NetBox API
// by calling GET /api/status/. It returns a descriptive error on any
// non-2xx response or transport-level failure.
func (c *Client) Ping(ctx context.Context) error {
	req, err := c.NewRequest(ctx, http.MethodGet, "/api/status/", nil)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("unable to reach netbox at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return fmt.Errorf("netbox rejected the API token (HTTP %d)", resp.StatusCode)
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("unexpected response from netbox status endpoint (HTTP %d): %s",
			resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	// Best-effort: drain a small amount so future keep-alive works cleanly.
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
	return nil
}

// Status is a minimal representation of the payload returned by the
// NetBox /api/status/ endpoint. Only fields likely to be useful for
// diagnostics are decoded; unknown fields are ignored.
type Status struct {
	NetboxVersion string `json:"netbox-version"`
	PythonVersion string `json:"python-version"`
}

// FetchStatus performs the same call as Ping but decodes and returns the
// parsed status payload. It is provided for future data sources.
func (c *Client) FetchStatus(ctx context.Context) (*Status, error) {
	req, err := c.NewRequest(ctx, http.MethodGet, "/api/status/", nil)
	if err != nil {
		return nil, err
	}
	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("unable to reach netbox at %s: %w", c.BaseURL, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return nil, fmt.Errorf("netbox /api/status/ returned HTTP %d: %s",
			resp.StatusCode, strings.TrimSpace(string(snippet)))
	}

	var s Status
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, fmt.Errorf("failed to decode netbox status response: %w", err)
	}
	return &s, nil
}

// DoJSON executes an HTTP request whose request and response bodies are
// JSON-encoded. If reqBody is nil, no request body is sent. If respBody is
// nil, the response body is discarded. A 404 response is translated to
// ErrNotFound. Any other non-2xx status returns an *APIError.
func (c *Client) DoJSON(ctx context.Context, method, path string, reqBody, respBody any) error {
	var body io.Reader
	if reqBody != nil {
		buf, err := json.Marshal(reqBody)
		if err != nil {
			return fmt.Errorf("netbox: failed to encode request body: %w", err)
		}
		body = bytes.NewReader(buf)
	}

	req, err := c.NewRequest(ctx, method, path, body)
	if err != nil {
		return err
	}

	resp, err := c.HTTPClient.Do(req)
	if err != nil {
		return fmt.Errorf("netbox: %s %s: %w", method, path, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode == http.StatusNotFound {
		// Drain a bit for keep-alive but preserve the sentinel error.
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<15))
		return ErrNotFound
	}

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		snippet, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
		return &APIError{
			StatusCode: resp.StatusCode,
			Status:     resp.Status,
			Method:     method,
			Path:       path,
			Body:       string(snippet),
		}
	}

	if respBody == nil || resp.StatusCode == http.StatusNoContent {
		_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, 1<<16))
		return nil
	}

	if err := json.NewDecoder(resp.Body).Decode(respBody); err != nil {
		return fmt.Errorf("netbox: failed to decode %s %s response: %w", method, path, err)
	}
	return nil
}
