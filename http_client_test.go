package main

import (
	"net/http"
	"testing"
)

func TestIsLoopbackHost(t *testing.T) {
	if !isLoopbackHost("127.0.0.1") {
		t.Fatal("127.0.0.1")
	}
	if !isLoopbackHost("localhost") {
		t.Fatal("localhost")
	}
	if !isLoopbackHost("::1") {
		t.Fatal("::1")
	}
	if !isLoopbackHost("0.0.0.0") {
		t.Fatal("0.0.0.0")
	}
	if isLoopbackHost("api.deepseek.com") {
		t.Fatal("deepseek should not be loopback")
	}
	if isLoopbackHost("192.168.1.1") {
		t.Fatal("lan should not be loopback")
	}
}

func TestSmartHTTPProxyBypassesLoopback(t *testing.T) {
	req, _ := http.NewRequest(http.MethodGet, "http://127.0.0.1:11434/v1/models", nil)
	u, err := smartHTTPProxy(req)
	if err != nil {
		t.Fatal(err)
	}
	if u != nil {
		t.Fatalf("expected no proxy for loopback, got %v", u)
	}
	req2, _ := http.NewRequest(http.MethodGet, "http://localhost:18080/v1/chat/completions", nil)
	u, err = smartHTTPProxy(req2)
	if err != nil || u != nil {
		t.Fatalf("localhost should bypass: %v %v", u, err)
	}
}

func TestClientFacingHost(t *testing.T) {
	if got := clientFacingHost("0.0.0.0"); got != "127.0.0.1" {
		t.Fatalf("%q", got)
	}
	if got := clientFacingHost("::"); got != "127.0.0.1" {
		t.Fatalf("%q", got)
	}
	if got := clientFacingHost("127.0.0.1"); got != "127.0.0.1" {
		t.Fatalf("%q", got)
	}
	if got := clientFacingHost("192.168.1.5"); got != "192.168.1.5" {
		t.Fatalf("%q", got)
	}
}

func TestFormatBaseURL(t *testing.T) {
	if got := formatBaseURL("0.0.0.0", 18080); got != "http://127.0.0.1:18080/v1" {
		t.Fatalf("%q", got)
	}
	if got := formatBaseURL("127.0.0.1", 18080); got != "http://127.0.0.1:18080/v1" {
		t.Fatalf("%q", got)
	}
}

func TestIsLocalProxyURL(t *testing.T) {
	if !isLocalProxyURL("http://127.0.0.1:18080/v1") {
		t.Fatal("expected local")
	}
	if !isLocalProxyURL("http://localhost:18080/v1") {
		t.Fatal("expected local localhost")
	}
	if isLocalProxyURL("https://api.deepseek.com/v1") {
		t.Fatal("cloud should not be local proxy url")
	}
}
