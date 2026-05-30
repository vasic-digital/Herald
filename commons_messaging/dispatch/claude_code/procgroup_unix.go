//go:build !windows

package claude_code

import (
	"os/exec"
	"syscall"
	"time"
)

// setProcessGroup places the child `claude` process in its own process
// group and installs a Cancel hook that SIGKILLs the WHOLE group when the
// command's context is cancelled (HRD-146).
//
// Rationale: exec.CommandContext's default cancellation only SIGKILLs the
// direct child. If `claude` ever spawns helper subprocesses that outlive a
// SIGKILL while still holding the stdout pipe write-end, cmd.Output() would
// stay blocked past the deadline (the well-known Go pipe-inheritance
// gotcha; flagged in HRD-127's FINDING note as the dispatcher-owner
// follow-up). Killing the group (kill(-pgid)) reaps the whole tree so the
// pipe is closed and Output() unblocks promptly.
//
// WaitDelay backstops the Cancel: if the group somehow lingers, Wait
// returns after the grace window rather than blocking forever.
func setProcessGroup(cmd *exec.Cmd) {
	if cmd == nil {
		return
	}
	if cmd.SysProcAttr == nil {
		cmd.SysProcAttr = &syscall.SysProcAttr{}
	}
	cmd.SysProcAttr.Setpgid = true
	// Backstop: if the group SIGKILL above doesn't unblock the stdout pipe
	// promptly, Wait returns after this grace window instead of hanging.
	cmd.WaitDelay = 2 * time.Second
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// Negative PID targets the whole process group. Best-effort: if the
		// group is already gone, the ESRCH is harmless.
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
