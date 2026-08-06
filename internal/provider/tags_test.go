package provider

import "testing"

func TestParseTagVersion(t *testing.T) {
	tests := []struct {
		in   string
		want []int
		ok   bool
	}{
		{"1.2.3", []int{1, 2, 3}, true},
		{"v1.2.3", []int{1, 2, 3}, true},
		{"V1.2.3", []int{1, 2, 3}, true},
		{"v403", []int{403}, true},
		{"1.2.3-beta", []int{1, 2, 3}, true},
		{"11.0.8.13000", []int{11, 0, 8, 13000}, true},
		{"release-2026-08-01", nil, false},
		{"latest", nil, false},
		{"", nil, false},
	}
	for _, tt := range tests {
		got, ok := parseTagVersion(tt.in)
		if ok != tt.ok {
			t.Errorf("parseTagVersion(%q) ok = %v, want %v", tt.in, ok, tt.ok)
			continue
		}
		if !ok {
			continue
		}
		if len(got) != len(tt.want) {
			t.Errorf("parseTagVersion(%q) = %v, want %v", tt.in, got, tt.want)
			continue
		}
		for i := range got {
			if got[i] != tt.want[i] {
				t.Errorf("parseTagVersion(%q) = %v, want %v", tt.in, got, tt.want)
				break
			}
		}
	}
}

func TestNewerTag(t *testing.T) {
	tests := []struct {
		a, b string
		want bool
	}{
		{"v1.10.0", "v1.9.0", true},
		{"v1.9.0", "v1.10.0", false},
		{"v2.0.0", "v1.99.99", true},
		{"1.2", "1.2.0", false},
		{"1.2.1", "1.2", true},
		{"v1.0.0", "weird-tag", true},
		{"weird-tag", "v1.0.0", false},
		{"b-tag", "a-tag", false},
	}
	for _, tt := range tests {
		if got := newerTag(tt.a, tt.b); got != tt.want {
			t.Errorf("newerTag(%q, %q) = %v, want %v", tt.a, tt.b, got, tt.want)
		}
	}
}

func TestPickTag(t *testing.T) {
	got := pickTag([]string{"v1.2.3", "v1.10.0", "v1.9.0"})
	if got != "v1.10.0" {
		t.Errorf("pickTag = %q, want v1.10.0", got)
	}

	got = pickTag([]string{"only-tag"})
	if got != "only-tag" {
		t.Errorf("pickTag = %q", got)
	}

	got = pickTag(nil)
	if got != "" {
		t.Errorf("pickTag(nil) = %q", got)
	}
}
