package provider

import "testing"

func TestManagerTracksEnabledOrder(t *testing.T) {
	m := NewManager(nil, []string{"wago", "github", "direct"}, map[string]bool{
		"wago":   true,
		"github": false,
		"direct": true,
	})
	if m.Enabled("github") {
		t.Error("github should be disabled")
	}
	m.SetEnabled("github", true)
	if !m.Enabled("github") {
		t.Error("github should be enabled after SetEnabled")
	}
	names := m.Names()
	if len(names) != 3 || names[0] != "direct" || names[1] != "github" || names[2] != "wago" {
		t.Errorf("names = %v", names)
	}
}
