package radiant

import (
	"context"
	"errors"
	"io"
	"os"
	"os/exec"
	"strconv"
	"strings"
	"syscall"
	"time"

	"golang.org/x/sys/windows"
)

const (
	pipeDir    = `\\.\pipe\`
	pipePrefix = "t7_radiant_bridge_"
)

type local struct{}

// Local is this computer.
var Local Machine = local{}

func (local) Bridges() ([]int, error) {
	entries, err := os.ReadDir(pipeDir)
	if err != nil {
		return nil, err
	}
	var pids []int
	for _, e := range entries {
		if s, ok := strings.CutPrefix(e.Name(), pipePrefix); ok {
			if pid, err := strconv.Atoi(s); err == nil {
				pids = append(pids, pid)
			}
		}
	}
	return pids, nil
}

// Dial opens the pipe for overlapped I/O: os.NewFile then ties it to the
// runtime's completion port, so a Read pending in the reader goroutine doesn't
// hold up a Write, as it would on a handle opened for synchronous I/O.
func (local) Dial(ctx context.Context, pid int) (io.ReadWriteCloser, error) {
	name := pipeDir + pipePrefix + strconv.Itoa(pid)
	p, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, err
	}
	for {
		h, err := windows.CreateFile(p, windows.GENERIC_READ|windows.GENERIC_WRITE, 0, nil,
			windows.OPEN_EXISTING, windows.FILE_FLAG_OVERLAPPED, 0)
		if err == nil {
			return os.NewFile(uintptr(h), name), nil
		}
		if !errors.Is(err, windows.ERROR_PIPE_BUSY) {
			return nil, err
		}
		// every instance is connected: the plugin opens the next one at once
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(20 * time.Millisecond):
		}
	}
}

// Start starts the program out of the caller's job when the job allows it:
// an MCP server's host may close its job, and every process in it, when it
// exits, and Radiant holds the user's work.
func (local) Start(exe, dir string, args ...string) (int, error) {
	start := func(flags uint32) (int, error) {
		c := exec.Command(exe, args...)
		c.Dir = dir
		c.SysProcAttr = &syscall.SysProcAttr{CreationFlags: flags}
		if err := c.Start(); err != nil {
			return 0, err
		}
		pid := c.Process.Pid
		return pid, c.Process.Release()
	}
	pid, err := start(windows.CREATE_BREAKAWAY_FROM_JOB)
	if errors.Is(err, windows.ERROR_ACCESS_DENIED) { // a job that forbids breakaway
		pid, err = start(0)
	}
	return pid, err
}

func (local) Running(pid int) bool {
	h, err := windows.OpenProcess(windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return false
	}
	defer func() { _ = windows.CloseHandle(h) }()
	ev, err := windows.WaitForSingleObject(h, 0)
	return err == nil && ev == uint32(windows.WAIT_TIMEOUT)
}
