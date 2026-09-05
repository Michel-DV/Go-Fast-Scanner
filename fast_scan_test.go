package main

import (
	"context"
	"net"
	"reflect"
	"testing"
	"time"
)

func TestParsePorts(t *testing.T) {
	tests := []struct {
		name string
		in   string
		want []int
	}{
		{name: "single", in: "22", want: []int{22}},
		{name: "list", in: "22,80,443", want: []int{22, 80, 443}},
		{name: "range", in: "1-4", want: []int{1, 2, 3, 4}},
		{name: "mixed", in: "22,80,443,8000-8002", want: []int{22, 80, 443, 8000, 8001, 8002}},
		{name: "deduplicate and sort", in: "443,80,80,79-81", want: []int{79, 80, 81, 443}},
		{name: "whitespace", in: " 22, 80-81 ,443 ", want: []int{22, 80, 81, 443}},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got, err := parsePorts(tt.in)
			if err != nil {
				t.Fatalf("parsePorts(%q) returned error: %v", tt.in, err)
			}
			if !reflect.DeepEqual(got, tt.want) {
				t.Fatalf("parsePorts(%q) = %v, want %v", tt.in, got, tt.want)
			}
		})
	}
}

func TestParsePortsRejectsInvalidInput(t *testing.T) {
	invalid := []string{
		"",
		"0",
		"65536",
		"80-79",
		"abc",
		"1--2",
		"80,",
		"-80",
		"80-",
	}

	for _, in := range invalid {
		t.Run(in, func(t *testing.T) {
			if _, err := parsePorts(in); err == nil {
				t.Fatalf("parsePorts(%q) expected error, got nil", in)
			}
		})
	}
}

func TestServiceName(t *testing.T) {
	if got := serviceName(22); got != "ssh" {
		t.Fatalf("serviceName(22) = %q, want %q", got, "ssh")
	}
	if got := serviceName(65000); got != "" {
		t.Fatalf("serviceName(65000) = %q, want empty string", got)
	}
}

func TestNormalizeTarget(t *testing.T) {
	tests := []struct {
		in      string
		want    string
		wantErr bool
	}{
		{in: "127.0.0.1", want: "127.0.0.1"},
		{in: "localhost", want: "localhost"},
		{in: "[::1]", want: "::1"},
		{in: "https://example.com", wantErr: true},
		{in: "example.com:443", wantErr: true},
		{in: "example.com/path", wantErr: true},
	}

	for _, tt := range tests {
		t.Run(tt.in, func(t *testing.T) {
			got, err := normalizeTarget(tt.in)
			if tt.wantErr {
				if err == nil {
					t.Fatalf("normalizeTarget(%q) expected error", tt.in)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeTarget(%q) returned error: %v", tt.in, err)
			}
			if got != tt.want {
				t.Fatalf("normalizeTarget(%q) = %q, want %q", tt.in, got, tt.want)
			}
		})
	}
}

func TestScanLocalListener(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("net.Listen: %v", err)
	}
	defer listener.Close()

	port := listener.Addr().(*net.TCPAddr).Port
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()

	open, scanned := scan(ctx, "127.0.0.1", []int{port}, 2, 250*time.Millisecond)
	if scanned != 1 {
		t.Fatalf("scanned = %d, want 1", scanned)
	}
	if len(open) != 1 || open[0].Port != port {
		t.Fatalf("open = %+v, want port %d", open, port)
	}
}
