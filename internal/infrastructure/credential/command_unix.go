//go:build unix

package credential

import (
	"os/exec"
	"syscall"
)

// boundProcess makes the timeout a real guarantee.
//
// Killing the program is not enough. exec.CommandContext kills the process it
// started, but a shell script's own children inherit the pipes rotator reads,
// so Wait keeps blocking on those pipes until the grandchild exits -- a
// `sleep 30` in a script outlived a 300ms timeout by the full 30 seconds.
//
// Two measures fix it: the child gets its own process group so cancellation
// can signal the whole tree, and WaitDelay bounds how long Wait will keep
// draining pipes afterwards before giving up on them.
func boundProcess(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}

	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		// A negative pid addresses the whole process group.
		if err := syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL); err != nil {
			// The group may already be gone; fall back to the process.
			return cmd.Process.Kill()
		}
		return nil
	}

	cmd.WaitDelay = processWaitDelay
}
