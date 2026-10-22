package build

import (
	"strings"
	"unsafe"

	"golang.org/x/sys/windows"
)

const gameExe = "BlackOps3.exe"

// Games are the running game's processes, as a log watcher asks after them:
// each one is opened when first listed, so its exit code can still be read
// once it has ended.
type Games struct {
	procs map[int]windows.Handle
}

// NewGames follows the game's processes.
func NewGames() *Games { return &Games{procs: map[int]windows.Handle{}} }

// Running are the pids of the game's processes.
func (g *Games) Running() []int {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var pids []int
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if !strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), gameExe) {
			continue
		}
		pid := int(pe.ProcessID)
		pids = append(pids, pid)
		if _, ok := g.procs[pid]; !ok {
			h, _ := windows.OpenProcess(windows.SYNCHRONIZE|windows.PROCESS_QUERY_LIMITED_INFORMATION, false, pe.ProcessID)
			g.procs[pid] = h
		}
	}
	return pids
}

// Dialogs are the dialogs the process shows, one line each, read as the run
// stage reads them: nothing is sent to the game until it shows one.
func (g *Games) Dialogs(pid int) []string {
	return dialogLines(gameWindows(pid))
}

// Exit is the exit code of a process Running listed, once it has ended; one
// that couldn't be opened ends with 0, unread.
func (g *Games) Exit(pid int) (uint32, bool) {
	h, ok := g.procs[pid]
	if !ok {
		return 0, true
	}
	if h != 0 {
		if ev, err := windows.WaitForSingleObject(h, 0); err == nil && ev == uint32(windows.WAIT_TIMEOUT) {
			return 0, false
		}
	}
	var code uint32
	if h != 0 {
		_ = windows.GetExitCodeProcess(h, &code)
		_ = windows.CloseHandle(h)
	}
	delete(g.procs, pid)
	return code, true
}
