package main

import "testing"

func TestNormalizedVersionBase(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name  string
		input string
		want  string
	}{
		{name: "plain tag", input: "v0.6.0", want: "0.6.0"},
		{name: "plain version without prefix", input: "0.6.0", want: "0.6.0"},
		{name: "git describe suffix", input: "v0.6.0-3-gabc1234", want: "0.6.0"},
		{name: "dirty git describe", input: "v0.6.0-3-gabc1234-dirty", want: "0.6.0"},
		{name: "uppercase prefix", input: "V0.6.0", want: "0.6.0"},
		{name: "blank", input: "   ", want: ""},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := normalizedVersionBase(test.input); got != test.want {
				t.Fatalf("normalizedVersionBase(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}

func TestHasVersionUpdate(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name           string
		currentVersion string
		latestVersion  string
		want           bool
	}{
		{name: "same version", currentVersion: "v0.6.0", latestVersion: "v0.6.0", want: false},
		{name: "same base with git describe suffix", currentVersion: "v0.6.0-5-gabc1234", latestVersion: "v0.6.0", want: false},
		{name: "same version with and without prefix", currentVersion: "0.6.0", latestVersion: "v0.6.0", want: false},
		{name: "newer latest version", currentVersion: "v0.5.0", latestVersion: "v0.6.0", want: true},
		{name: "dev current version", currentVersion: "dev", latestVersion: "v0.6.0", want: true},
		{name: "missing latest version", currentVersion: "v0.6.0", latestVersion: "", want: false},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()
			if got := hasVersionUpdate(test.currentVersion, test.latestVersion); got != test.want {
				t.Fatalf("hasVersionUpdate(%q, %q) = %v, want %v", test.currentVersion, test.latestVersion, got, test.want)
			}
		})
	}
}
