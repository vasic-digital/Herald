//go:build windows

package claude_code

import "os/exec"

// setProcessGroup is a no-op on platforms without POSIX process groups.
// exec.CommandContext's default cancellation (SIGKILL of the direct child)
// still applies, so the per-message deadline is honoured; only the
// child-subprocess-holding-the-pipe edge case (HRD-146 note) is unaddressed
// here. Herald's runtime targets Unix hosts.
func setProcessGroup(cmd *exec.Cmd) {}
