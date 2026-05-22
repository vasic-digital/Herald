package claude_code

import (
	"context"
	"os/exec"
)

// BuildCmdForTest exposes the internal buildCmd helper so unit tests can
// inspect the constructed *exec.Cmd (argv, dir, env) without spawning the
// `claude` binary. Same-package _test.go visibility per Go convention.
func (d *Dispatcher) BuildCmdForTest(ctx context.Context, req DispatchRequest) (*exec.Cmd, error) {
	cmd, _, _, err := d.buildCmd(ctx, req)
	return cmd, err
}

// WorkingDirForTest exposes the dispatcher's working directory so the
// argv-inspection test can pre-create the session anchor file under it
// (the test never runs the binary, it only resolves the session UUID).
func (d *Dispatcher) WorkingDirForTest() string { return d.workingDir }
