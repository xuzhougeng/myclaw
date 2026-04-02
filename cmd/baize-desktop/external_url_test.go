package main

import "testing"

func TestNormalizeExternalURL(t *testing.T) {
	t.Parallel()

	tests := []struct {
		name    string
		input   string
		want    string
		wantErr bool
	}{
		{name: "https url", input: "https://github.com/xuzhougeng/baize/releases", want: "https://github.com/xuzhougeng/baize/releases"},
		{name: "trim spaces", input: "  http://example.com/path  ", want: "http://example.com/path"},
		{name: "reject blank", input: "   ", wantErr: true},
		{name: "reject missing host", input: "https:///releases", wantErr: true},
		{name: "reject file scheme", input: "file:///tmp/test", wantErr: true},
	}

	for _, test := range tests {
		test := test
		t.Run(test.name, func(t *testing.T) {
			t.Parallel()

			got, err := normalizeExternalURL(test.input)
			if test.wantErr {
				if err == nil {
					t.Fatalf("normalizeExternalURL(%q) expected error", test.input)
				}
				return
			}
			if err != nil {
				t.Fatalf("normalizeExternalURL(%q) unexpected error: %v", test.input, err)
			}
			if got != test.want {
				t.Fatalf("normalizeExternalURL(%q) = %q, want %q", test.input, got, test.want)
			}
		})
	}
}
