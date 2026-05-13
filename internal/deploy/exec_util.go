package deploy

import (
	"os/exec"
	"syscall"
)

// groupKill configures cmd so ctx cancellation SIGKILLs the entire process
// group. Without this, a child like `docker pull` whose grandchildren hold
// stdout/stderr open leaves cmd.Wait() blocked until they exit naturally.
//
// Shared by docker.go and update_container.go — both shell out to host
// binaries that may spawn subprocesses.
func groupKill(cmd *exec.Cmd) {
	cmd.SysProcAttr = &syscall.SysProcAttr{Setpgid: true}
	cmd.Cancel = func() error {
		if cmd.Process == nil {
			return nil
		}
		return syscall.Kill(-cmd.Process.Pid, syscall.SIGKILL)
	}
}
