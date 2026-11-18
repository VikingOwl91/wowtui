// Package sandbox runs the private archive worker and validates its output.
package sandbox

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"somegit.dev/vikingowl/wowtui/internal/config"
	"somegit.dev/vikingowl/wowtui/internal/installer"
	"somegit.dev/vikingowl/wowtui/internal/sandbox/worker"
)

type launch struct {
	backend, warning string
}

// Extract starts a separate worker for one archive. Capability selection ends
// before any archive data is processed; an extraction failure is final.
func Extract(ctx context.Context, policy config.Security, archive, staging string, limits installer.Limits, report func(backend, warning string)) error {
	for _, mode := range []string{policy.Sandbox, policy.Seccomp} {
		if mode != "" && mode != "auto" && mode != "required" {
			return errors.New("invalid extraction security policy")
		}
	}
	self, err := os.Executable()
	if err != nil {
		return err
	}
	archive, err = filepath.Abs(archive)
	if err != nil {
		return err
	}
	staging, err = filepath.Abs(staging)
	if err != nil {
		return err
	}

	run, err := prepare(ctx, self, policy, archive, staging)
	if err != nil {
		return err
	}
	if report != nil {
		report(run.backend, run.warning)
	}
	if err := run.extract(ctx, self, archive, staging, limits); err != nil {
		return err
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return worker.Validate(staging, limits)
}

func direct(ctx context.Context, self, archive, staging string, limits installer.Limits) error {
	cmd := exec.CommandContext(ctx, self, worker.Argument)
	cmd.Dir = staging
	cmd.Env = []string{}
	return worker.Run(ctx, cmd, archive, staging, limits)
}

func withoutSandbox(policy config.Security, reason string) (launch, error) {
	if policy.Sandbox == "required" || policy.Seccomp == "required" {
		return launch{}, fmt.Errorf("required extraction sandbox unavailable: %s", reason)
	}
	return launch{backend: "worker", warning: reason}, nil
}
