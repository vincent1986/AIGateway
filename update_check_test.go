package main

import "testing"

func TestCompareVersions(t *testing.T) {
	tests := []struct {
		name string
		a    string
		b    string
		want int
	}{
		{name: "newer patch", a: "2.0.10", b: "2.0.1", want: 1},
		{name: "same with v prefix", a: "v2.0.1", b: "2.0.1", want: 0},
		{name: "older minor", a: "2.0.1", b: "2.1.0", want: -1},
		{name: "missing patch equals zero patch", a: "2.0", b: "2.0.0", want: 0},
		{name: "pre release suffix ignored", a: "2.0.2-beta.1", b: "2.0.1", want: 1},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := compareVersions(tt.a, tt.b)
			if got != tt.want {
				t.Fatalf("compareVersions(%q, %q) = %d, want %d", tt.a, tt.b, got, tt.want)
			}
		})
	}
}
