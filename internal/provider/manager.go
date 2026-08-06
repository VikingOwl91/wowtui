package provider

import (
	"context"
	"sort"

	"somegit.dev/vikingowl/wowtui/internal/wow"
)

type Manager struct {
	providers map[string]Provider
	order     []string
	enabled   map[string]bool
}

func NewManager(providers map[string]Provider, order []string, enabled map[string]bool) *Manager {
	if enabled == nil {
		enabled = make(map[string]bool)
	}
	return &Manager{providers: providers, order: append([]string(nil), order...), enabled: enabled}
}

func (m *Manager) Enabled(name string) bool {
	return m.enabled[name]
}

func (m *Manager) SetEnabled(name string, enabled bool) {
	m.enabled[name] = enabled
}

func (m *Manager) Active() []Provider {
	active := make([]Provider, 0, len(m.order))
	for _, name := range m.order {
		if m.enabled[name] {
			if p, ok := m.providers[name]; ok {
				active = append(active, p)
			}
		}
	}
	return active
}

func (m *Manager) Names() []string {
	names := append([]string(nil), m.order...)
	sort.Strings(names)
	return names
}

func (m *Manager) Search(ctx context.Context, query string, flavor wow.GameFlavor) ([]RemoteAddon, error) {
	var results []RemoteAddon
	for _, p := range m.Active() {
		searcher, ok := p.(SearchProvider)
		if !ok {
			continue
		}
		found, err := searcher.Search(ctx, query, flavor)
		if err != nil {
			return nil, err
		}
		results = append(results, found...)
	}
	return results, nil
}

func (m *Manager) Match(ctx context.Context, hint MatchHint, flavor wow.GameFlavor) ([]RemoteAddon, error) {
	var results []RemoteAddon
	for _, p := range m.Active() {
		matcher, ok := p.(MatchProvider)
		if !ok {
			continue
		}
		found, matched, err := matcher.Match(ctx, hint, flavor)
		if err != nil {
			return nil, err
		}
		if matched {
			results = append(results, found)
		}
	}
	return results, nil
}
