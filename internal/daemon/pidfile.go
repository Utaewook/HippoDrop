package daemon

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"syscall"

	"hippodrop/internal/project"
)

func WritePIDFile(projectName string) error {
	pidPath := project.PIDFilePath(projectName)
	if data, err := os.ReadFile(pidPath); err == nil {
		var pid int
		if _, scanErr := fmt.Sscanf(string(data), "%d", &pid); scanErr == nil {
			if IsProcessRunning(pid) {
				return fmt.Errorf("hippodrop daemon is already running for project '%s' (PID: %d)", projectName, pid)
			}
		}
	}

	if err := os.MkdirAll(filepath.Dir(pidPath), 0755); err != nil {
		return err
	}

	return os.WriteFile(pidPath, []byte(fmt.Sprintf("%d", os.Getpid())), 0644)
}

func RemovePIDFile(projectName string) {
	_ = os.Remove(project.PIDFilePath(projectName))
}

func IsProcessRunning(pid int) bool {
	proc, err := os.FindProcess(pid)
	if err != nil {
		return false
	}
	return IsProcessAlive(proc)
}

func IsProcessAlive(p *os.Process) bool {
	if runtime.GOOS == "windows" {
		err := p.Signal(os.Interrupt)
		return err == nil || !errors.Is(err, os.ErrProcessDone)
	}
	err := p.Signal(syscall.Signal(0))
	return err == nil
}
