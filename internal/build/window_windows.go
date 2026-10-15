//go:build windows

package build

import (
	"sync"
	"unsafe"

	"golang.org/x/sys/windows"
)

var sendMessageTimeout = windows.NewLazySystemDLL("user32.dll").NewProc("SendMessageTimeoutW")

const (
	wmGetText       = 0x000D
	wmGetTextLength = 0x000E
	smtoAbortIfHung = 0x0002
	textTimeoutMS   = 500
	maxWindowText   = 1 << 20 // the console holds the game's whole log
)

// The enumeration callbacks, made once (Go can make only so many), collect
// into these under enumMu.
var (
	enumMu   sync.Mutex
	enumPID  uint32
	enumTop  []windows.HWND
	enumKids []windows.HWND
	topProc  = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		var pid uint32
		if _, err := windows.GetWindowThreadProcessId(h, &pid); err == nil && pid == enumPID && windows.IsWindowVisible(h) {
			enumTop = append(enumTop, h)
		}
		return 1
	})
	kidProc = windows.NewCallback(func(h windows.HWND, _ uintptr) uintptr {
		enumKids = append(enumKids, h)
		return 1
	})
)

// processWindows lists process pid's visible top-level windows by class and,
// once one is a dialog, reads the dialog's and the console's text: until
// then nothing is sent to the game, whose window answers WM_GETTEXT on the
// thread that loads the map.
func processWindows(pid int) []window {
	enumMu.Lock()
	defer enumMu.Unlock()
	enumPID, enumTop = uint32(pid), nil
	_ = windows.EnumWindows(topProc, nil)
	out := make([]window, len(enumTop))
	dialog := false
	for i, h := range enumTop {
		out[i].Class = className(h)
		dialog = dialog || out[i].Class == dialogClass
	}
	for i, h := range enumTop {
		if dialog && (out[i].Class == dialogClass || out[i].Class == consoleClass) {
			out[i].Title, out[i].Controls = windowText(h), controls(h)
		}
	}
	return out
}

// controls are a window's child controls that have text.
func controls(h windows.HWND) []control {
	enumKids = nil
	windows.EnumChildWindows(h, kidProc, nil)
	var out []control
	for _, k := range enumKids {
		if t := windowText(k); t != "" {
			out = append(out, control{className(k), t})
		}
	}
	return out
}

func className(h windows.HWND) string {
	buf := make([]uint16, 256)
	n, err := windows.GetClassName(h, &buf[0], int32(len(buf)))
	if err != nil {
		return ""
	}
	return windows.UTF16ToString(buf[:n])
}

// windowText asks a window for its text with WM_GETTEXT, as GetWindowText
// reads no other process's controls; a hung window gets textTimeoutMS.
func windowText(h windows.HWND) string {
	var n uintptr
	if !send(h, wmGetTextLength, 0, 0, &n) || n == 0 {
		return ""
	}
	n = min(n, maxWindowText)
	buf := make([]uint16, n+1)
	var got uintptr
	if !send(h, wmGetText, n+1, uintptr(unsafe.Pointer(&buf[0])), &got) {
		return ""
	}
	return windows.UTF16ToString(buf[:min(got, n)])
}

func send(h windows.HWND, msg uint32, w, l uintptr, result *uintptr) bool {
	r, _, _ := sendMessageTimeout.Call(uintptr(h), uintptr(msg), w, l, smtoAbortIfHung, textTimeoutMS, uintptr(unsafe.Pointer(result)))
	return r != 0
}
