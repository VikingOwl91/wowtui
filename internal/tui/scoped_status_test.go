package tui

import (
	"testing"

	"somegit.dev/vikingowl/wowtui/internal/registry"
	"somegit.dev/vikingowl/wowtui/internal/service"
)

func TestCheckDistinguishesSameSourceInTwoClients(t *testing.T) {
	m := testModel(t)
	m.rows = []addonRow{
		{Managed: true, Ref: "owner/addon", InstallPath: "/retail"},
		{Managed: true, Ref: "owner/addon", InstallPath: "/classic"},
	}
	m.mergeStatuses([]service.UpdateStatus{
		{Addon: registry.Addon{Ref: "owner/addon", InstallPath: "/retail"}, HasUpdate: true},
		{Addon: registry.Addon{Ref: "owner/addon", InstallPath: "/classic"}},
	})
	if m.rows[0].Status != "update" || m.rows[1].Status != "current" {
		t.Fatalf("distinct client statuses: got %q and %q", m.rows[0].Status, m.rows[1].Status)
	}
}
