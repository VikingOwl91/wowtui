//go:build !linux

package sandbox

import (
	"context"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/installer"
)

type runner struct{ launch }

func prepare(_ context.Context, _ string, policy config.Security, _, _ string) (runner, error) {
	selected, err := withoutSandbox(policy, "Bubblewrap is only available on Linux")
	return runner{selected}, err
}

func (r runner) extract(ctx context.Context, self, archive, staging string, limits installer.Limits) error {
	return direct(ctx, self, archive, staging, limits)
}
