//go:build !unix

package credential

import "os/exec"

// boundProcess bounds a command on platforms without process groups.
//
// Only WaitDelay is available here, so a cancelled command's children may
// outlive it; Wait still returns once the delay elapses, which is what keeps
// the daemon from wedging.
func boundProcess(cmd *exec.Cmd) {
	cmd.WaitDelay = processWaitDelay
}
