// Package worker implements the private extraction protocol shared by the
// launcher and the worker invocation of the wowtui binary.
package worker

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os/exec"
	"time"

	"somegit.dev/vikingowl/wowtui/internal/installer"
)

const (
	Argument      = "--internal-extract"
	ProbeArgument = "--internal-extract-probe"
	messageLimit  = 64 << 10
)

// Dispatch handles the private invocations before any dotenv, CLI or test
// initialization. args excludes the executable name.
func Dispatch(args []string, in io.Reader, out, diagnostics io.Writer) (handled bool, code int) {
	if len(args) != 1 || (args[0] != ProbeArgument && args[0] != Argument) {
		return false, 0
	}
	if err := CloseInherited(); err != nil {
		fmt.Fprintln(diagnostics, "isolate extraction worker descriptors:", err)
		return true, 1
	}
	if args[0] == Argument {
		if err := Serve(in, out); err != nil {
			fmt.Fprintln(diagnostics, "extraction worker protocol:", err)
			return true, 1
		}
	}
	return true, 0
}

type request struct {
	Archive string           `json:"archive"`
	Staging string           `json:"staging"`
	Limits  installer.Limits `json:"limits"`
}

type reply struct {
	OK    bool   `json:"ok"`
	Error string `json:"error,omitempty"`
}

func decode(data []byte, dest any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.DisallowUnknownFields()
	if err := dec.Decode(dest); err != nil {
		return err
	}
	if err := dec.Decode(new(json.RawMessage)); err != io.EOF {
		return errors.New("trailing data in worker message")
	}
	return nil
}

func validLimits(limits installer.Limits) bool {
	return limits.MaxFiles > 0 && limits.MaxFileSize > 0 && limits.MaxTotalSize > 0
}

// Serve processes exactly one request and exits. An archive error is carried
// in the reply so callers do not have to parse process diagnostics.
func Serve(in io.Reader, out io.Writer) error {
	data, err := io.ReadAll(io.LimitReader(in, messageLimit+1))
	if err != nil {
		return err
	}
	if len(data) > messageLimit {
		return errors.New("worker request exceeds size limit")
	}
	var req request
	if err := decode(data, &req); err != nil {
		return fmt.Errorf("invalid worker request: %w", err)
	}
	if req.Archive == "" || req.Staging == "" || !validLimits(req.Limits) {
		return errors.New("worker request needs archive, staging and positive limits")
	}

	result := reply{OK: true}
	if err := installer.ExtractZip(req.Archive, req.Staging, req.Limits); err != nil {
		result = reply{Error: err.Error()}
	}
	response, err := json.Marshal(result)
	if err != nil {
		return err
	}
	if len(response) > messageLimit {
		response = []byte(`{"ok":false,"error":"archive extraction failed (diagnostic exceeds size limit)"}`)
	}
	_, err = out.Write(response)
	return err
}

// cappedOutput bounds retained bytes, even if the worker returns malformed
// or excessive output. Returning an error closes the command's output pipe.
type cappedOutput struct {
	buf bytes.Buffer
}

func (out *cappedOutput) Write(p []byte) (int, error) {
	if len(p) > messageLimit-out.buf.Len() {
		return 0, errors.New("worker reply exceeds size limit")
	}
	return out.buf.Write(p)
}

// Run exchanges a single request with a command built with CommandContext.
// archive and staging use the worker's path namespace. Call Validate on the
// host staging path after this returns successfully.
func Run(ctx context.Context, cmd *exec.Cmd, archive, staging string, limits installer.Limits) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if !validLimits(limits) {
		return errors.New("extraction requires positive limits")
	}
	data, err := json.Marshal(request{Archive: archive, Staging: staging, Limits: limits})
	if err != nil {
		return err
	}
	if len(data) > messageLimit {
		return errors.New("worker request exceeds size limit")
	}
	if cmd.Env == nil {
		cmd.Env = []string{}
	}
	cmd.Stdin = bytes.NewReader(data)
	var output cappedOutput
	cmd.Stdout = &output
	cmd.WaitDelay = 2 * time.Second
	err = cmd.Run()
	if ctx.Err() != nil {
		return ctx.Err()
	}
	if err != nil {
		return fmt.Errorf("extraction worker: %w", err)
	}
	var result reply
	if err := decode(output.buf.Bytes(), &result); err != nil {
		return fmt.Errorf("invalid extraction worker reply: %w", err)
	}
	switch {
	case result.OK && result.Error == "":
		return nil
	case !result.OK && result.Error != "":
		return fmt.Errorf("extraction worker: %s", result.Error)
	default:
		return errors.New("inconsistent extraction worker reply")
	}
}
