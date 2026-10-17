package radiant

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strconv"
	"testing"
	"time"

	"golang.org/x/sys/windows"

	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

// pipeBridge serves a fake Radiant on a real \\.\pipe\t7_radiant_bridge_<pid>,
// as the plugin does: one overlapped instance per connection.
func pipeBridge(t *testing.T, f *radianttest.Fake) {
	t.Helper()
	name, _ := windows.UTF16PtrFromString(pipeDir + pipePrefix + strconv.Itoa(f.PID))
	h, err := windows.CreateNamedPipe(name, windows.PIPE_ACCESS_DUPLEX|windows.FILE_FLAG_OVERLAPPED|windows.FILE_FLAG_FIRST_PIPE_INSTANCE,
		windows.PIPE_TYPE_BYTE|windows.PIPE_WAIT|windows.PIPE_REJECT_REMOTE_CLIENTS, 1, 1<<16, 1<<16, 0, nil)
	if err != nil {
		t.Fatal(err)
	}
	server := os.NewFile(uintptr(h), "server")
	t.Cleanup(func() { server.Close() })
	go func() {
		ev, _ := windows.CreateEvent(nil, 1, 0, nil)
		defer func() { _ = windows.CloseHandle(ev) }()
		ov := windows.Overlapped{HEvent: ev}
		switch err := windows.ConnectNamedPipe(h, &ov); err {
		case windows.ERROR_IO_PENDING:
			_, _ = windows.WaitForSingleObject(ev, windows.INFINITE)
		case nil, windows.ERROR_PIPE_CONNECTED:
		default:
			return
		}
		fake := f.Dial()
		go func() { _, _ = io.Copy(fake, server) }()
		_, _ = io.Copy(server, fake)
	}()
}

func TestLocalTalksToABridgeOverARealPipe(t *testing.T) {
	f := radianttest.New(2_000_000_000+os.Getpid(), `C:\a.map`)
	f.Handle("echo", func(_ *radianttest.Fake, args string, body []byte) radianttest.Reply {
		return radianttest.OK(args, body)
	})
	pipeBridge(t, f)
	pids, err := Local.Bridges()
	if err != nil || !slices.Contains(pids, f.PID) {
		t.Fatalf("bridges = %v, %v: the test's pipe is missing", pids, err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	c, err := Connect(ctx, Local, f.PID) // hello is written while the reader's read is pending
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Call(ctx, "subscribe console", nil); err != nil {
		t.Fatal(err)
	}
	big := make([]byte, 3<<20) // larger than the pipe's buffers: written while events come in
	for i := range big {
		big[i] = byte(i)
	}
	done := make(chan struct{})
	go func() {
		defer close(done)
		for range 50 {
			f.Event("console", "line")
		}
	}()
	r, err := c.Call(ctx, "echo big", big)
	if err != nil || r.Summary != "big" || !slices.Equal(r.Body, big) {
		t.Fatalf("echo of %d bytes: summary %q, %d bytes back, %v", len(big), r.Summary, len(r.Body), err)
	}
	<-done
	st, err := c.State(ctx)
	if err != nil || st.Map != `C:\a.map` {
		t.Errorf("state = %+v, %v", st, err)
	}
}

func TestLocalDialOfNoBridgeFailsAtOnce(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	start := time.Now()
	if _, err := Local.Dial(ctx, 2_100_000_000+os.Getpid()); err == nil || time.Since(start) > 2*time.Second {
		t.Errorf("dial of no pipe: %v after %s", err, time.Since(start))
	}
}

func TestLocalStartsAProgramAndSeesItRun(t *testing.T) {
	if !Local.Running(os.Getpid()) {
		t.Error("this test's own process is not running")
	}
	if Local.Running(2_100_000_000) {
		t.Error("a pid nobody has is running")
	}
	exe, err := exec.LookPath("cmd.exe")
	if err != nil {
		t.Skip("no cmd.exe")
	}
	pid, err := Local.Start(exe, filepath.Dir(exe), "/c", "exit")
	if err != nil || pid <= 0 {
		t.Fatalf("start = %d, %v", pid, err)
	}
	deadline := time.Now().Add(5 * time.Second)
	for Local.Running(pid) && time.Now().Before(deadline) {
		time.Sleep(10 * time.Millisecond)
	}
	if Local.Running(pid) {
		t.Errorf("cmd /c exit (pid %d) still runs", pid)
	}
}
