package radiant_test

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"net"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/McReaper/t7_companion/internal/radiant"
	"github.com/McReaper/t7_companion/internal/radiant/radianttest"
)

func ctx(t *testing.T) context.Context {
	t.Helper()
	c, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return c
}

func connect(t *testing.T, f *radianttest.Fake) *radiant.Client {
	t.Helper()
	c, err := radiant.NewClient(ctx(t), f.Dial())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { c.Close() })
	return c
}

func TestHelloReadsWhatThePluginSaysOfItself(t *testing.T) {
	f := radianttest.New(4242, `C:\bo3\map_source\a.map`)
	f.Plugin = "1.2.3"
	c := connect(t, f)
	if want := (radiant.Hello{Plugin: "1.2.3", Host: "1.1.0", PID: 4242}); c.Hello != want {
		t.Errorf("hello = %+v, want %+v", c.Hello, want)
	}
	if got := f.Requests(); len(got) != 1 || got[0] != "hello 1" {
		t.Errorf("requests = %q, want hello 1 first", got)
	}
}

func TestAPluginOfAnotherProtocolIsRefusedByName(t *testing.T) {
	f := radianttest.New(1, "")
	f.Handle("hello", func(*radianttest.Fake, string, []byte) radianttest.Reply {
		return radianttest.Err("version", "this plugin serves protocol 2")
	})
	_, err := radiant.NewClient(ctx(t), f.Dial())
	if err == nil || !strings.Contains(err.Error(), "does not speak protocol 1") || !radiant.IsCode(err, "version") {
		t.Errorf("err = %v, want the protocol named and the version code kept", err)
	}
}

func TestCallReturnsTheSummaryAndBodyOrAnErrorWithItsCode(t *testing.T) {
	f := radianttest.New(1, "")
	f.Handle("echo", func(_ *radianttest.Fake, args string, body []byte) radianttest.Reply {
		if args == "fail" {
			return radianttest.Err("not-found", "{GUID} is gone")
		}
		return radianttest.OK("got "+args, append([]byte("body:"), body...))
	})
	c := connect(t, f)
	r, err := c.Call(ctx(t), "echo two words", []byte("x\ny"))
	if err != nil || r.Summary != "got two words" || string(r.Body) != "body:x\ny" {
		t.Errorf("echo = %+v, %v", r, err)
	}
	long := strings.Repeat("map text ", 200) // a body much longer than its header line
	if r, err := c.Call(ctx(t), "echo long", []byte(long)); err != nil || string(r.Body) != "body:"+long {
		t.Errorf("echo of %d bytes = %d bytes, %v", len(long), len(r.Body), err)
	}
	_, err = c.Call(ctx(t), "echo fail", nil)
	var e *radiant.Error
	if !errors.As(err, &e) || *e != (radiant.Error{Command: "echo", Code: "not-found", Message: "{GUID} is gone"}) {
		t.Errorf("err = %#v", err)
	}
	if err.Error() != "Radiant echo: not-found: {GUID} is gone" || !radiant.IsCode(err, "not-found") || radiant.IsCode(err, "busy") {
		t.Errorf("err text or code check wrong: %v", err)
	}
	if radiant.IsCode(errors.New("plain"), "busy") {
		t.Error("a plain error has no code")
	}
	if _, err := c.Call(ctx(t), "echo a\nb", nil); err == nil || !strings.Contains(err.Error(), "line break") {
		t.Errorf("a line break in the request line: err = %v", err)
	}
}

func TestAnOKWithoutSummaryAndAnUnknownCommand(t *testing.T) {
	f := radianttest.New(1, "")
	c := connect(t, f)
	if r, err := c.Call(ctx(t), "subscribe map", nil); err != nil || r.Summary != "" || len(r.Body) != 0 {
		t.Errorf("subscribe = %+v, %v", r, err)
	}
	if _, err := c.Call(ctx(t), "frobnicate", nil); !radiant.IsCode(err, "bad-request") {
		t.Errorf("unknown command: err = %v", err)
	}
}

func TestADialogMakesEditsBusyWhileStateStillAnswers(t *testing.T) {
	f := radianttest.New(1, `C:\m.map`)
	c := connect(t, f)
	f.Update(func(f *radianttest.Fake) { f.Dialog = "Save As" })
	if _, err := c.Call(ctx(t), "reload C:\\m.map", nil); !radiant.IsCode(err, "busy") || !strings.Contains(err.Error(), "Save As") {
		t.Errorf("reload under a dialog: err = %v", err)
	}
	st, err := c.State(ctx(t))
	if err != nil || st.Dialog != "Save As" || st.Map != `C:\m.map` {
		t.Errorf("state under a dialog = %+v, %v", st, err)
	}
}

func TestStateReadsEveryKey(t *testing.T) {
	f := radianttest.New(1, `D:\BO3\map_source\zm\zm_a.map`)
	f.Modified, f.PrefabDepth, f.Entities, f.Selected, f.Minimised = true, 2, 31, 4, true
	f.Render, f.View2D = "Fullbright", "YZ"
	st, err := connect(t, f).State(ctx(t))
	want := radiant.State{Map: `D:\BO3\map_source\zm\zm_a.map`, Modified: true, PrefabDepth: 2, Entities: 31, Selected: 4,
		Minimised: true, Render: "Fullbright", View2D: "YZ"}
	if err != nil || st != want {
		t.Errorf("state = %+v, %v\nwant %+v", st, err, want)
	}
}

func TestStateIgnoresANewerPluginsKeysAndCRLF(t *testing.T) {
	f := radianttest.New(1, "")
	f.Handle("state", func(*radianttest.Fake, string, []byte) radianttest.Reply {
		return radianttest.OK("", []byte("map=C:\\a.map\r\nmodified=0\r\nlayer=Zones\r\nselected=3\r\n"))
	})
	st, err := connect(t, f).State(ctx(t))
	if err != nil || st != (radiant.State{Map: `C:\a.map`, Selected: 3}) {
		t.Errorf("state = %+v, %v", st, err)
	}
}

func TestRepliesReachTheirCallersWhenCallsOverlap(t *testing.T) {
	f := radianttest.New(1, "")
	f.Handle("echo", func(_ *radianttest.Fake, args string, _ []byte) radianttest.Reply { return radianttest.OK(args, nil) })
	c := connect(t, f)
	var wg sync.WaitGroup
	for i := range 20 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			r, err := c.Call(ctx(t), fmt.Sprint("echo ", i), nil)
			if err != nil || r.Summary != fmt.Sprint(i) {
				t.Errorf("call %d = %+v, %v", i, r, err)
			}
		}()
	}
	wg.Wait()
}

func TestEventsArriveBetweenReplies(t *testing.T) {
	f := radianttest.New(1, "")
	c := connect(t, f)
	if _, err := c.Call(ctx(t), "subscribe console map", nil); err != nil {
		t.Fatal(err)
	}
	f.Event("console", "All Probes Done (3.2 s)")
	f.Event("map", "saved C:\\a b.map")
	f.Event("other", "not subscribed")
	for _, want := range []radiant.Event{{Name: "console", Args: "All Probes Done (3.2 s)"}, {Name: "map", Args: `saved C:\a b.map`}} {
		select {
		case e := <-c.Events():
			if e != want {
				t.Errorf("event = %+v, want %+v", e, want)
			}
		case <-time.After(2 * time.Second):
			t.Fatalf("no event %+v", want)
		}
	}
	if _, err := c.Call(ctx(t), "state", nil); err != nil {
		t.Errorf("a call after events: %v", err)
	}
}

func TestEventsCloseWithTheConnection(t *testing.T) {
	c := connect(t, radianttest.New(1, ""))
	c.Close()
	select {
	case _, open := <-c.Events():
		if open {
			t.Error("an event after close")
		}
	case <-time.After(2 * time.Second):
		t.Fatal("events stay open after close")
	}
	if _, err := c.Call(ctx(t), "state", nil); err == nil || !strings.Contains(err.Error(), "closed") {
		t.Errorf("a call after close: err = %v", err)
	}
}

// rawBridge serves a connection by hand: fn gets each request's id and line
// and writes what it likes, to test what the fake never sends.
func rawBridge(t *testing.T, fn func(w io.Writer, id, line string) bool) io.ReadWriteCloser {
	t.Helper()
	client, server := net.Pipe()
	go func() {
		defer func() { _ = server.Close() }()
		for {
			var n [4]byte
			if _, err := io.ReadFull(server, n[:]); err != nil {
				return
			}
			b := make([]byte, binary.LittleEndian.Uint32(n[:]))
			if _, err := io.ReadFull(server, b); err != nil {
				return
			}
			head, _, _ := strings.Cut(string(b), "\n")
			id, line, _ := strings.Cut(head, " ")
			if !fn(server, id, line) {
				return
			}
		}
	}()
	return client
}

func frame(w io.Writer, payload string) {
	_, _ = w.Write(append(binary.LittleEndian.AppendUint32(nil, uint32(len(payload))), payload...))
}

func helloThen(then func(w io.Writer, id, line string) bool) func(w io.Writer, id, line string) bool {
	return func(w io.Writer, id, line string) bool {
		if line == "hello 1" {
			frame(w, id+" ok 1 plugin=1.0.0 host=1.1.0 pid=7\n")
			return true
		}
		return then(w, id, line)
	}
}

func TestABrokenBridgeEndsTheConnectionWithTheReason(t *testing.T) {
	for name, tc := range map[string]struct {
		send func(w io.Writer, id string)
		want string
	}{
		"closed":         {func(io.Writer, string) {}, "Radiant closed, or crashed"},
		"no id":          {func(w io.Writer, _ string) { frame(w, "ok\n") }, "without an id"},
		"unsent id":      {func(w io.Writer, _ string) { frame(w, "99 ok\n") }, "request 99, which was never sent"},
		"neither":        {func(w io.Writer, id string) { frame(w, id+" maybe\n") }, "neither ok nor err"},
		"okay is not ok": {func(w io.Writer, id string) { frame(w, id+" okay\n") }, "neither ok nor err"},
		"huge frame": {func(w io.Writer, _ string) {
			_, _ = w.Write(binary.LittleEndian.AppendUint32(nil, 1<<20+1))
		}, "a 1048577-byte frame, more than 1048576"},
		"unreadable request": {func(w io.Writer, _ string) {
			frame(w, "0 err bad-request a request starts with its id\n")
		}, "the bridge could not read a request: bad-request a request starts with its id"},
	} {
		t.Run(name, func(t *testing.T) {
			t.Cleanup(radiant.SetMaxFrame(1 << 20))
			c, err := radiant.NewClient(ctx(t), rawBridge(t, helloThen(func(w io.Writer, id, _ string) bool {
				tc.send(w, id)
				return false
			})))
			if err != nil {
				t.Fatal(err)
			}
			defer c.Close()
			if _, err := c.Call(ctx(t), "state", nil); err == nil || !strings.Contains(err.Error(), tc.want) {
				t.Errorf("err = %v, want %q", err, tc.want)
			}
			if _, err := c.Call(ctx(t), "state", nil); err == nil {
				t.Error("the connection still serves after it broke")
			}
		})
	}
}

func TestAFrameAtTheCapIsRead(t *testing.T) {
	t.Cleanup(radiant.SetMaxFrame(64))
	c, err := radiant.NewClient(ctx(t), rawBridge(t, helloThen(func(w io.Writer, id, _ string) bool {
		head := id + " ok "
		frame(w, head+strings.Repeat("s", 64-len(head)))
		return true
	})))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if r, err := c.Call(ctx(t), "state", nil); err != nil || len(r.Summary) != 64-len("2 ok ") {
		t.Errorf("a 64-byte frame under a 64-byte cap = %q, %v", r.Summary, err)
	}
}

func TestRequestIDsCountFromOne(t *testing.T) {
	var ids []string
	c, err := radiant.NewClient(ctx(t), rawBridge(t, func(w io.Writer, id, _ string) bool {
		ids = append(ids, id)
		frame(w, id+" ok\n")
		return true
	}))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	if _, err := c.Call(ctx(t), "state", nil); err != nil {
		t.Fatal(err)
	}
	if !slices.Equal(ids, []string{"1", "2"}) {
		t.Errorf("ids = %q, want 1 then 2 (the plugin reads 1 to 18 digits)", ids)
	}
}

func TestAReplyGivenUpOnDoesNotBreakTheConnection(t *testing.T) {
	var late string
	c, err := radiant.NewClient(ctx(t), rawBridge(t, helloThen(func(w io.Writer, id, line string) bool {
		if line == "slow" {
			late = id // answered with the next request, like a capture still waiting for its frame
			return true
		}
		frame(w, late+" ok slow done\n")
		frame(w, id+" ok fast\n")
		return true
	})))
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	short, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if _, err := c.Call(short, "slow", nil); !errors.Is(err, context.DeadlineExceeded) || !strings.Contains(err.Error(), "no answer from Radiant to slow") {
		t.Errorf("slow: err = %v", err)
	}
	if r, err := c.Call(ctx(t), "fast", nil); err != nil || r.Summary != "fast" {
		t.Errorf("fast after a slow one given up on = %+v, %v", r, err)
	}
}

func TestAFailedHelloClosesTheConnection(t *testing.T) {
	_, err := radiant.NewClient(ctx(t), rawBridge(t, func(w io.Writer, id, _ string) bool {
		frame(w, id+" err bad-request hello <protocol>\n")
		return true
	}))
	if !radiant.IsCode(err, "bad-request") {
		t.Errorf("err = %v", err)
	}
}

func TestAWriteThatFailsEndsTheCall(t *testing.T) {
	client, server := net.Pipe()
	_ = server.Close()
	if _, err := radiant.NewClient(ctx(t), client); err == nil {
		t.Error("hello over a closed pipe succeeded")
	}
}

func TestSamePathIgnoresCaseAndSlashes(t *testing.T) {
	for _, tc := range []struct {
		a, b string
		want bool
	}{
		{`D:\BO3\map_source\a.map`, `d:/bo3/map_source/A.MAP`, true},
		{`D:\BO3\map_source\zm\..\a.map`, `D:\BO3\map_source\a.map`, true},
		{`D:\BO3\map_source\a.map`, `D:\BO3\map_source\b.map`, false},
		{"", "", false},
		{`C:\a.map`, "", false},
	} {
		if got := radiant.SamePath(tc.a, tc.b); got != tc.want {
			t.Errorf("SamePath(%q, %q) = %v", tc.a, tc.b, got)
		}
	}
}
