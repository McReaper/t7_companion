//go:build windows

package build

import (
	"os"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
	"unsafe"

	"golang.org/x/sys/windows"
)

var (
	testUser32    = windows.NewLazySystemDLL("user32.dll")
	createWindow  = testUser32.NewProc("CreateWindowExW")
	destroyWindow = testUser32.NewProc("DestroyWindow")
	findWindow    = testUser32.NewProc("FindWindowW")
	postMessage   = testUser32.NewProc("PostMessageW")
)

// A window of this process standing for the game's own, and a real message
// box, the kind the game shows its fatal errors in: processWindows lists the
// plain window by its class alone, sending it nothing, and reads the box's
// title, message and button, then the box is closed.
func TestProcessWindowsReadsAMessageBoxAndOnlyIt(t *testing.T) {
	plain := openPlainWindow(t, "t7kb plain window")
	if ws := processWindows(os.Getpid()); !slices.Equal(classes(ws), []string{"Static"}) || ws[0].Title != "" || ws[0].Controls != nil {
		t.Fatalf("with no dialog, the plain window alone and unread: %+v", ws)
	}

	title := "t7kb test " + time.Now().Format("150405.000")
	msg := strings.Repeat("ERROR: Could not find zone 'fr_zm_test' ", 3) + "é"
	done := make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(done)
		t16, _ := windows.UTF16PtrFromString(title)
		m16, _ := windows.UTF16PtrFromString(msg)
		_, _ = windows.MessageBox(0, m16, t16, windows.MB_OK|windows.MB_ICONERROR)
	}()
	var box *window
	var seen []window
	for deadline := time.Now().Add(5 * time.Second); box == nil && time.Now().Before(deadline); time.Sleep(20 * time.Millisecond) {
		seen = processWindows(os.Getpid())
		for _, w := range seen {
			if w.Class == dialogClass && w.Title == title {
				box = &w
			}
		}
	}
	closeBox(t, title)
	<-done
	if box == nil {
		t.Fatalf("no message box among this process's windows %+v", seen)
	}
	want := `the game opened a dialog, "` + title + `": ` + strings.TrimSpace(msg) + " [OK]"
	if got := dialogLine(*box); got != want {
		t.Errorf("%q\nwant %q\nfrom %+v", got, want, box.Controls)
	}
	for _, w := range seen {
		if w.Class != dialogClass && (w.Title != "" || w.Controls != nil) {
			t.Errorf("a window that is no dialog or console was read: %+v", w)
		}
	}
	plain()
	if ws := processWindows(os.Getpid()); len(ws) != 0 {
		t.Errorf("once both are closed, this process has windows %+v", ws)
	}
	if ws := processWindows(0); len(ws) != 0 {
		t.Errorf("process 0 has windows %+v", ws)
	}
}

func classes(ws []window) []string {
	var out []string
	for _, w := range ws {
		out = append(out, w.Class)
	}
	return out
}

// openPlainWindow opens a visible top-level Static window off the screen, on
// a thread of its own as Windows wants, and returns what closes it.
func openPlainWindow(t *testing.T, text string) (closeIt func()) {
	t.Helper()
	made, quit, closed := make(chan uintptr), make(chan struct{}), make(chan struct{})
	go func() {
		runtime.LockOSThread()
		defer close(closed)
		cls, _ := windows.UTF16PtrFromString("STATIC")
		txt, _ := windows.UTF16PtrFromString(text)
		const wsPopup, wsVisible = 0x80000000, 0x10000000
		off := -32000
		h, _, _ := createWindow.Call(0, uintptr(unsafe.Pointer(cls)), uintptr(unsafe.Pointer(txt)), wsPopup|wsVisible,
			uintptr(off), uintptr(off), 10, 10, 0, 0, 0, 0)
		made <- h
		<-quit
		_, _, _ = destroyWindow.Call(h)
	}()
	if h := <-made; h == 0 {
		t.Fatal("no window was made")
	}
	return func() { close(quit); <-closed }
}

func closeBox(t *testing.T, title string) {
	t.Helper()
	t16, _ := windows.UTF16PtrFromString(title)
	h, _, _ := findWindow.Call(0, uintptr(unsafe.Pointer(t16)))
	if h == 0 {
		t.Error("the box can't be found to close it")
		return
	}
	const wmClose = 0x0010
	_, _, _ = postMessage.Call(h, wmClose, 0, 0)
}
