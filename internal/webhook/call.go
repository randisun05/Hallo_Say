// Package webhook lets the agent make outbound HTTP calls on the user's
// behalf, with basic guardrails against hitting internal/private network
// targets from a shared or cloud-hosted deployment.
package webhook

import (
	"fmt"
	"io"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const maxResponseBytes = 8 * 1024

var allowedMethods = map[string]bool{
	http.MethodGet:    true,
	http.MethodPost:   true,
	http.MethodPut:    true,
	http.MethodPatch:  true,
	http.MethodDelete: true,
}

func Call(rawURL, method, body string) (string, error) {
	if method == "" {
		method = http.MethodGet
	}
	method = strings.ToUpper(method)
	if !allowedMethods[method] {
		return "", fmt.Errorf("method %q not allowed", method)
	}

	parsed, err := url.Parse(rawURL)
	if err != nil {
		return "", fmt.Errorf("invalid url: %w", err)
	}
	if parsed.Scheme != "http" && parsed.Scheme != "https" {
		return "", fmt.Errorf("only http/https urls are allowed")
	}
	if err := rejectPrivateHost(parsed.Hostname()); err != nil {
		return "", err
	}

	var reqBody io.Reader
	if body != "" {
		reqBody = strings.NewReader(body)
	}

	req, err := http.NewRequest(method, parsed.String(), reqBody)
	if err != nil {
		return "", fmt.Errorf("build request: %w", err)
	}
	if body != "" {
		req.Header.Set("content-type", "application/json")
	}

	client := &http.Client{Timeout: 10 * time.Second}
	resp, err := client.Do(req)
	if err != nil {
		return "", fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	limited := io.LimitReader(resp.Body, maxResponseBytes)
	respBody, err := io.ReadAll(limited)
	if err != nil {
		return "", fmt.Errorf("read response: %w", err)
	}

	return fmt.Sprintf("status: %d\nbody: %s", resp.StatusCode, string(respBody)), nil
}

func rejectPrivateHost(host string) error {
	ips, err := net.LookupIP(host)
	if err != nil {
		return fmt.Errorf("resolve host: %w", err)
	}
	for _, ip := range ips {
		if ip.IsLoopback() || ip.IsPrivate() || ip.IsLinkLocalUnicast() || ip.IsUnspecified() {
			return fmt.Errorf("target host resolves to a disallowed address")
		}
	}
	return nil
}
