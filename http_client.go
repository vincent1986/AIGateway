package main

import (
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"
)

// newHTTPTransport returns a transport that:
//   - uses the OS / environment HTTP(S)_PROXY for remote upstreams (common on Windows)
//   - NEVER proxies loopback / unspecified hosts (127.0.0.1, localhost, ::1, 0.0.0.0)
//
// Without the loopback bypass, Windows system proxy (Clash / corporate proxy) often
// breaks Ollama (127.0.0.1:11434) and can mis-route local AI Switch proxy traffic.
func newHTTPTransport() *http.Transport {
	return &http.Transport{
		Proxy:                 smartHTTPProxy,
		ProxyConnectHeader:    make(http.Header),
		MaxIdleConns:          32,
		IdleConnTimeout:       90 * time.Second,
		TLSHandshakeTimeout:   15 * time.Second,
		ExpectContinueTimeout: 1 * time.Second,
		// Windows dual-stack: prefer happy eyeballs defaults
		ForceAttemptHTTP2: true,
	}
}

// newHTTPClient builds a client with smart proxy + optional timeout (0 = none).
func newHTTPClient(timeout time.Duration) *http.Client {
	return &http.Client{
		Timeout:   timeout,
		Transport: newHTTPTransport(),
	}
}

// smartHTTPProxy respects HTTP_PROXY / HTTPS_PROXY / NO_PROXY from the environment
// for remote hosts, but forces direct connect for loopback addresses.
func smartHTTPProxy(req *http.Request) (*url.URL, error) {
	if req == nil || req.URL == nil {
		return nil, nil
	}
	if isLoopbackHost(req.URL.Hostname()) {
		return nil, nil
	}
	return http.ProxyFromEnvironment(req)
}

func isLoopbackHost(host string) bool {
	host = strings.TrimSpace(strings.ToLower(host))
	if host == "" {
		return false
	}
	// strip zone id if present (fe80::1%eth0)
	if i := strings.IndexByte(host, '%'); i >= 0 {
		host = host[:i]
	}
	host = strings.Trim(host, "[]")
	switch host {
	case "localhost", "0.0.0.0", "::", "::1":
		return true
	}
	ip := net.ParseIP(host)
	if ip == nil {
		// hostname that might resolve to loopback — only treat explicit localhost
		return host == "localhost" || strings.HasSuffix(host, ".localhost")
	}
	return ip.IsLoopback() || ip.IsUnspecified()
}

// clientFacingHost converts a listen bind address into a URL host clients can use.
// Binding 0.0.0.0 / :: is valid for Listen, but http://0.0.0.0:port is not a usable client URL on Windows.
func clientFacingHost(bindHost string) string {
	h := strings.TrimSpace(bindHost)
	h = strings.Trim(h, "[]")
	if h == "" {
		return "127.0.0.1"
	}
	if h == "0.0.0.0" || h == "::" {
		return "127.0.0.1"
	}
	if ip := net.ParseIP(h); ip != nil && ip.IsUnspecified() {
		return "127.0.0.1"
	}
	return h
}

// formatBaseURL builds http://host:port/v1 for Codex / UI.
func formatBaseURL(bindHost string, port int) string {
	host := clientFacingHost(bindHost)
	// IPv6 literal needs brackets
	if ip := net.ParseIP(host); ip != nil && ip.To4() == nil {
		return fmt.Sprintf("http://[%s]:%d/v1", host, port)
	}
	return fmt.Sprintf("http://%s:%d/v1", host, port)
}
