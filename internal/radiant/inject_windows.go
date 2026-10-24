package radiant

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	kernel32            = windows.NewLazySystemDLL("kernel32.dll")
	user32              = windows.NewLazySystemDLL("user32.dll")
	pVirtualAllocEx     = kernel32.NewProc("VirtualAllocEx")
	pVirtualFreeEx      = kernel32.NewProc("VirtualFreeEx")
	pCreateRemoteThread = kernel32.NewProc("CreateRemoteThread")
	pGetExitCodeThread  = kernel32.NewProc("GetExitCodeThread")
	pWaitForInputIdle   = user32.NewProc("WaitForInputIdle")
	pFindWindowExW      = user32.NewProc("FindWindowExW")
	pGetWindowThreadPID = user32.NewProc("GetWindowThreadProcessId")
	pSendMessageTimeout = user32.NewProc("SendMessageTimeoutW")
)

const radiantExe = "radiant_modtools.exe"

func (local) Radiants() ([]int, error) {
	snap, err := windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPPROCESS, 0)
	if err != nil {
		return nil, err
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var pids []int
	var pe windows.ProcessEntry32
	pe.Size = uint32(unsafe.Sizeof(pe))
	for err = windows.Process32First(snap, &pe); err == nil; err = windows.Process32Next(snap, &pe) {
		if strings.EqualFold(windows.UTF16ToString(pe.ExeFile[:]), radiantExe) {
			pids = append(pids, int(pe.ProcessID))
		}
	}
	return pids, nil
}

// Modules snapshots the process's modules, again while it is still loading
// them: the snapshot of a process that just started fails with
// ERROR_PARTIAL_COPY or ERROR_BAD_LENGTH until its loader settles.
func (local) Modules(pid int) ([]string, error) {
	var snap windows.Handle
	var err error
	for range 40 {
		snap, err = windows.CreateToolhelp32Snapshot(windows.TH32CS_SNAPMODULE|windows.TH32CS_SNAPMODULE32, uint32(pid))
		if !errors.Is(err, windows.ERROR_PARTIAL_COPY) && !errors.Is(err, windows.ERROR_BAD_LENGTH) {
			break
		}
		time.Sleep(25 * time.Millisecond)
	}
	if err != nil {
		return nil, fmt.Errorf("the modules of pid %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(snap) }()
	var out []string
	var me windows.ModuleEntry32
	me.Size = uint32(unsafe.Sizeof(me))
	for err = windows.Module32First(snap, &me); err == nil; err = windows.Module32Next(snap, &me) {
		out = append(out, windows.UTF16ToString(me.ExePath[:]))
	}
	return out, nil
}

// Ready waits until the process takes input: until then Radiant has no main
// window for the bridge to find.
func (local) Ready(ctx context.Context, pid int) error {
	h, err := windows.OpenProcess(windows.PROCESS_QUERY_LIMITED_INFORMATION|windows.SYNCHRONIZE, false, uint32(pid))
	if err != nil {
		return err
	}
	defer func() { _ = windows.CloseHandle(h) }()
	for {
		switch r, _, e := pWaitForInputIdle.Call(uintptr(h), 250); r {
		case 0:
			return nil
		case uintptr(windows.WAIT_TIMEOUT):
		default:
			return fmt.Errorf("waiting for pid %d to take input: %w", pid, e)
		}
		if err := ctx.Err(); err != nil {
			return err
		}
	}
}

// Inject runs LoadLibraryW(dll) in the process, from a thread it starts there.
func (local) Inject(pid int, dll string) error {
	const access = windows.PROCESS_CREATE_THREAD | windows.PROCESS_QUERY_INFORMATION |
		windows.PROCESS_VM_OPERATION | windows.PROCESS_VM_WRITE | windows.PROCESS_VM_READ | windows.SYNCHRONIZE
	h, err := windows.OpenProcess(access, false, uint32(pid))
	if err != nil {
		return fmt.Errorf("open pid %d: %w", pid, err)
	}
	defer func() { _ = windows.CloseHandle(h) }()
	path, err := windows.UTF16FromString(dll)
	if err != nil {
		return err
	}
	size := uintptr(len(path) * 2)
	remote, _, e := pVirtualAllocEx.Call(uintptr(h), 0, size, windows.MEM_COMMIT|windows.MEM_RESERVE, windows.PAGE_READWRITE)
	if remote == 0 {
		return fmt.Errorf("VirtualAllocEx: %w", e)
	}
	free := func() { _, _, _ = pVirtualFreeEx.Call(uintptr(h), remote, 0, windows.MEM_RELEASE) }
	var written uintptr
	if err := windows.WriteProcessMemory(h, remote, (*byte)(unsafe.Pointer(&path[0])), size, &written); err != nil {
		free()
		return fmt.Errorf("WriteProcessMemory: %w", err)
	}
	// kernel32 sits at the same address in every process of a boot
	loadLibrary, err := windows.GetProcAddress(windows.Handle(kernel32.Handle()), "LoadLibraryW")
	if err != nil {
		free()
		return err
	}
	th, _, e := pCreateRemoteThread.Call(uintptr(h), 0, 0, loadLibrary, remote, 0, 0)
	if th == 0 {
		free()
		return fmt.Errorf("CreateRemoteThread: %w", e)
	}
	defer func() { _ = windows.CloseHandle(windows.Handle(th)) }()
	if ev, err := windows.WaitForSingleObject(windows.Handle(th), 15000); err != nil || ev != windows.WAIT_OBJECT_0 {
		return fmt.Errorf("the remote LoadLibraryW of %s did not return within 15 s", dll) // its path stays allocated: the thread may still read it
	}
	free()
	var code uint32
	if ok, _, e := pGetExitCodeThread.Call(th, uintptr(unsafe.Pointer(&code))); ok == 0 {
		return fmt.Errorf("GetExitCodeThread: %w", e)
	}
	if code == 0 {
		return fmt.Errorf("the remote LoadLibraryW of %s failed", dll)
	}
	return nil
}

// Control sends one command to MTPlugins' control window in the process (a
// message-only window of class RadiantPlugins_Control) and returns the reply
// the host writes to bin/plugins/RadiantPlugins.ctl.
func (l local) Control(pid int, cmd string) (string, error) {
	win, err := controlWindow(pid)
	if err != nil {
		return "", err
	}
	mods, err := l.Modules(pid)
	if err != nil || len(mods) == 0 {
		return "", fmt.Errorf("the path of pid %d: %v", pid, err)
	}
	ctl := filepath.Join(filepath.Dir(mods[0]), "plugins", "RadiantPlugins.ctl")
	before, _ := os.Stat(ctl)
	b := []byte(cmd)
	cds := struct {
		data uintptr
		size uint32
		ptr  uintptr
	}{size: uint32(len(b)), ptr: uintptr(unsafe.Pointer(&b[0]))}
	const wmCopyData, smtoAbortIfHung = 0x004A, 0x0002
	var result uintptr
	if r, _, e := pSendMessageTimeout.Call(win, wmCopyData, 0, uintptr(unsafe.Pointer(&cds)), smtoAbortIfHung, 10000,
		uintptr(unsafe.Pointer(&result))); r == 0 {
		return "", fmt.Errorf("%s: %w", cmd, e)
	}
	for range 40 { // the host writes its reply after it handled the message
		if after, err := os.Stat(ctl); err == nil && (before == nil || !after.ModTime().Equal(before.ModTime()) || after.Size() != before.Size()) {
			break
		}
		time.Sleep(50 * time.Millisecond)
	}
	reply, err := os.ReadFile(ctl)
	return strings.TrimSpace(string(reply)), err
}

// controlWindow is MTPlugins' control window in the process.
func controlWindow(pid int) (uintptr, error) {
	class, err := windows.UTF16PtrFromString("RadiantPlugins_Control")
	if err != nil {
		return 0, err
	}
	const hwndMessage = ^uintptr(2) // HWND_MESSAGE, (HWND)-3
	var after uintptr
	for {
		h, _, _ := pFindWindowExW.Call(hwndMessage, after, uintptr(unsafe.Pointer(class)), 0)
		if h == 0 {
			return 0, errors.New("MTPlugins' control window isn't in that Radiant")
		}
		var owner uint32
		_, _, _ = pGetWindowThreadPID.Call(h, uintptr(unsafe.Pointer(&owner)))
		if int(owner) == pid {
			return h, nil
		}
		after = h
	}
}
