// Package radianttest is a Radiant with the T7RadiantBridge plugin, in
// memory, for tests on any OS: a Fake serves the bridge protocol over the
// connections a test opens, and a Machine is a radiant.Machine of fakes. It is
// written from PROTOCOL.md, not from the client, so it checks the client's
// framing too: a request before hello, an unknown command, a request under a
// dialog get the plugin's errors.
package radianttest

import (
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"strings"
	"sync"
	"time"
)

// Reply is a command's answer: ok when Code is "".
type Reply struct {
	Summary string
	Body    []byte
	Code    string
	Message string
}

// OK is an ok reply.
func OK(summary string, body []byte) Reply { return Reply{Summary: summary, Body: body} }

// Err is an err reply.
func Err(code, message string) Reply { return Reply{Code: code, Message: message} }

// Handler serves one command, with the Fake locked: args is the request line
// after the command.
type Handler func(f *Fake, args string, body []byte) Reply

// Fake is one Radiant. Its exported fields are its state: set them before the
// test connects, or through Update while it serves.
type Fake struct {
	PID         int
	Plugin      string // the plugin version hello reports
	Map         string
	Modified    bool
	PrefabDepth int
	Entities    int
	Minimised   bool
	Dialog      string // a modal dialog's title: every command but hello, state and menus is busy
	Render      string
	View2D      string
	LoadAfter   time.Duration     // how long reload takes to show the new map in state
	Text        string            // the map's entities, as entities gives them
	Selection   []string          // the selected brushes' GUIDs
	Menus       map[string]string // menus' entries by path, as menus gives them: "label	enabled	submenu" lines
	UndoLabel   string            // the undo menu's entry: what the last edit was labelled
	views

	mu       sync.Mutex
	handlers map[string]Handler
	requests []string
	conns    map[*conn]bool
	made     int // GUIDs given
}

type conn struct {
	net.Conn
	wmu  sync.Mutex
	subs map[string]bool
}

// New is a Radiant with the plugin's commands and the map open.
func New(pid int, mapPath string) *Fake {
	f := &Fake{PID: pid, Plugin: "1.0.0", Map: mapPath, Render: "Real Lighting", View2D: "XY (Top)",
		views: views{W3: 160, H3: 90, Focal: 120, W2: 90, H2: 160, Scale2D: 1}, conns: map[*conn]bool{}}
	f.handlers = map[string]Handler{"hello": hello, "state": state, "reload": reload, "subscribe": subscribe,
		"entities": entities, "selection": selection, "camera": camera, "capture": capture, "command": command, "action": action,
		"edit": edit, "select": selectCmd, "deselect": deselect, "menus": menus}
	return f
}

// Handle adds a command, or replaces one.
func (f *Fake) Handle(command string, h Handler) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.handlers[command] = h
}

// Update changes the fake's state while it serves.
func (f *Fake) Update(fn func(*Fake)) {
	f.mu.Lock()
	defer f.mu.Unlock()
	fn(f)
}

// Requests are the request lines served (command and arguments), in order.
func (f *Fake) Requests() []string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return append([]string(nil), f.requests...)
}

// Dial opens a connection to the fake's bridge.
func (f *Fake) Dial() io.ReadWriteCloser {
	client, server := net.Pipe()
	c := &conn{Conn: server, subs: map[string]bool{}}
	f.mu.Lock()
	f.conns[c] = true
	f.mu.Unlock()
	go f.serve(c)
	return client
}

// Event sends an event to every connection subscribed to it.
func (f *Fake) Event(name, args string) {
	f.mu.Lock()
	var to []*conn
	for c := range f.conns {
		if c.subs[name] {
			to = append(to, c)
		}
	}
	f.mu.Unlock()
	for _, c := range to {
		_ = c.send("0 " + strings.TrimSpace(name+" "+args) + "\n") // a closed connection is the reader's to notice
	}
}

func (f *Fake) serve(c *conn) {
	defer func() {
		f.mu.Lock()
		delete(f.conns, c)
		f.mu.Unlock()
		_ = c.Close()
	}()
	greeted := false
	for {
		payload, err := readFrame(c)
		if err != nil {
			return
		}
		head, body, _ := strings.Cut(string(payload), "\n")
		id, line, _ := strings.Cut(head, " ")
		if why := badID(id); why != "" { // the plugin answers with id 0, having none to answer with
			if c.send("0 err bad-request "+why+"\n") != nil {
				return
			}
			continue
		}
		command, args, _ := strings.Cut(line, " ")
		r := f.run(c, greeted, command, args, []byte(body), line)
		greeted = greeted || (command == "hello" && r.Code == "")
		h := id + " ok"
		if r.Code != "" {
			h = id + " err " + r.Code + " " + r.Message
		} else if r.Summary != "" {
			h += " " + r.Summary
		}
		if c.send(h+"\n"+string(r.Body)) != nil {
			return
		}
	}
}

// badID is why the plugin can't read a request's id: 1 to 18 digits, not 0.
func badID(id string) string {
	if id == "" || len(id) > 18 || strings.Trim(id, "0123456789") != "" {
		return "a request starts with its id"
	}
	if strings.Trim(id, "0") == "" {
		return "request ids start at 1 (0 is for events)"
	}
	return ""
}

func (f *Fake) run(c *conn, greeted bool, command, args string, body []byte, line string) Reply {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.requests = append(f.requests, line)
	h := f.handlers[command]
	switch {
	case !greeted && command != "hello":
		return Err("bad-request", "hello first")
	case h == nil:
		return Err("bad-request", "unknown command "+command)
	case f.Dialog != "" && command != "hello" && command != "state" && command != "menus":
		return Err("busy", f.Dialog)
	case command == "subscribe":
		for _, e := range strings.Fields(args) {
			c.subs[e] = true
		}
	}
	return h(f, args, body)
}

func (c *conn) send(payload string) error {
	frame := binary.LittleEndian.AppendUint32(nil, uint32(len(payload)))
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err := c.Write(append(frame, payload...))
	return err
}

func readFrame(r io.Reader) ([]byte, error) {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	b := make([]byte, binary.LittleEndian.Uint32(n[:]))
	_, err := io.ReadFull(r, b)
	return b, err
}

func hello(f *Fake, args string, _ []byte) Reply {
	if args != "1" {
		return Err("version", "this plugin serves protocol 1")
	}
	return OK(fmt.Sprintf("1 plugin=%s host=1.1.0 pid=%d", f.Plugin, f.PID), nil)
}

func state(f *Fake, _ string, _ []byte) Reply {
	b := func(v bool) int {
		if v {
			return 1
		}
		return 0
	}
	entities, selected := fmt.Sprint(f.Entities), fmt.Sprint(len(f.Selection))
	if f.Dialog != "" {
		entities, selected = "", ""
	}
	return OK("", fmt.Appendf(nil, "map=%s\nmodified=%d\nprefab_depth=%d\nentities=%s\nselected=%s\nminimised=%d\ndialog=%s\nrender=%s\nview2d=%s\n",
		f.Map, b(f.Modified), f.PrefabDepth, entities, selected, b(f.Minimised), f.Dialog, f.Render, f.View2D))
}

// reload opens the map without a prompt, as the plugin does: the path must
// exist, and the new map shows after LoadAfter.
func reload(f *Fake, args string, _ []byte) Reply {
	path := strings.TrimSpace(args)
	switch {
	case path == "":
		return Err("bad-request", "reload <path…>")
	case f.PrefabDepth > 0:
		return Err("refused", "leave the prefab first")
	}
	if _, err := os.Stat(path); err != nil {
		return Err("not-found", path)
	}
	open := func() { f.Map, f.Modified, f.Selection = path, false, nil }
	if f.LoadAfter == 0 {
		open()
	} else {
		time.AfterFunc(f.LoadAfter, func() { f.Update(func(*Fake) { open() }) })
	}
	return OK("", nil)
}

func subscribe(_ *Fake, args string, _ []byte) Reply {
	if strings.TrimSpace(args) == "" {
		return Err("bad-request", "subscribe <event>… (console, map)")
	}
	return OK("", nil)
}
