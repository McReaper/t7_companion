package radiant

import (
	"context"
	"encoding/binary"
	"errors"
	"fmt"
	"io"
	"strconv"
	"strings"
	"sync"
)

// Protocol is the bridge protocol this client speaks.
const Protocol = 1

// MinPlugin is the oldest plugin serving every command this client sends: the
// first release, 1.2.0.
const MinPlugin = "1.2.0"

// maxFrame bounds a frame from the bridge: a capture of a large view or a
// whole map's text is tens of MiB, a corrupt length is not.
var maxFrame uint32 = 512 << 20

// Reply is an ok reply: its summary (the header after "ok") and its body.
type Reply struct {
	Summary string
	Body    []byte
}

// Error is an err reply: the code PROTOCOL.md lists (busy, not-found,
// refused…) and Radiant's or the plugin's message.
type Error struct {
	Command, Code, Message string
}

func (e *Error) Error() string {
	return fmt.Sprintf("Radiant %s: %s: %s", e.Command, e.Code, e.Message)
}

// IsCode reports whether err is an err reply with that code.
func IsCode(err error, code string) bool {
	var e *Error
	return errors.As(err, &e) && e.Code == code
}

// Event is one event the bridge sent: its name (console, map) and the rest of
// its header.
type Event struct {
	Name, Args string
}

// Hello is what the plugin said about itself.
type Hello struct {
	Plugin, Host string // versions
	PID          int
}

// Client is one connection to a Radiant's bridge.
type Client struct {
	Hello Hello

	rw     io.ReadWriteCloser
	events chan Event
	wmu    sync.Mutex // one frame on the wire at a time

	mu      sync.Mutex
	next    uint64
	pending map[uint64]chan result
	err     error // why the connection ended, once it has
}

type result struct {
	r   Reply
	err error
}

// NewClient starts a connection over rw, saying hello first. It closes rw when
// the hello fails.
func NewClient(ctx context.Context, rw io.ReadWriteCloser) (*Client, error) {
	c := &Client{rw: rw, events: make(chan Event, 256), pending: map[uint64]chan result{}}
	go c.read()
	r, err := c.Call(ctx, "hello "+strconv.Itoa(Protocol), nil)
	if err != nil {
		c.Close()
		if IsCode(err, "version") {
			return nil, fmt.Errorf("this Radiant's bridge plugin does not speak protocol %d: update t7kb or the plugin (%w)", Protocol, err)
		}
		return nil, err
	}
	c.Hello = parseHello(r.Summary)
	if !versionAtLeast(c.Hello.Plugin, MinPlugin) {
		c.Close()
		return nil, fmt.Errorf("this Radiant's bridge is %q, older than %s, the first one with every command t7kb sends: close Radiant, then open the map again for t7kb to load its own", c.Hello.Plugin, MinPlugin)
	}
	return c, nil
}

// versionAtLeast reports whether the dotted version v is min or later, a
// missing part counting as 0; a part that isn't a number is never later.
func versionAtLeast(v, min string) bool {
	vs, ms := strings.Split(v, "."), strings.Split(min, ".")
	for i, m := range ms {
		have := 0
		if i < len(vs) {
			n, err := strconv.Atoi(vs[i])
			if err != nil {
				return false
			}
			have = n
		}
		want, _ := strconv.Atoi(m)
		if have != want {
			return have > want
		}
	}
	return true
}

// parseHello reads "1 plugin=1.2.0 host=1.2.0 pid=123".
func parseHello(s string) Hello {
	var h Hello
	for _, kv := range strings.Fields(s) {
		k, v, _ := strings.Cut(kv, "=")
		switch k {
		case "plugin":
			h.Plugin = v
		case "host":
			h.Host = v
		case "pid":
			h.PID, _ = strconv.Atoi(v)
		}
	}
	return h
}

// Call sends one request, its header line (the command and its arguments) and
// body, and waits for its reply. An err reply is an *Error.
func (c *Client) Call(ctx context.Context, line string, body []byte) (Reply, error) {
	if strings.ContainsAny(line, "\r\n") {
		return Reply{}, fmt.Errorf("a request line cannot hold a line break: %q", line)
	}
	command, _, _ := strings.Cut(line, " ")
	c.mu.Lock()
	if c.err != nil {
		err := c.err
		c.mu.Unlock()
		return Reply{}, err
	}
	c.next++
	id := c.next
	ch := make(chan result, 1) // the reader never blocks on a caller that gave up
	c.pending[id] = ch
	c.mu.Unlock()

	payload := make([]byte, 0, len(line)+len(body)+24)
	payload = strconv.AppendUint(payload, id, 10)
	payload = append(append(append(payload, ' '), line...), '\n')
	if err := c.write(append(payload, body...)); err != nil {
		c.fail(err)
		return Reply{}, c.gone()
	}
	select {
	case res := <-ch:
		var e *Error
		if errors.As(res.err, &e) {
			e.Command = command
		}
		return res.r, res.err
	case <-ctx.Done():
		return Reply{}, fmt.Errorf("no answer from Radiant to %s: %w", command, ctx.Err())
	}
}

// Events are the events this connection subscribed to, until it closes. One
// that arrives while the channel is full is dropped.
func (c *Client) Events() <-chan Event { return c.events }

// Close ends the connection; requests still queued in Radiant are dropped.
func (c *Client) Close() {
	c.fail(errors.New("the connection to Radiant is closed"))
}

func (c *Client) write(payload []byte) error {
	frame := make([]byte, 4, 4+len(payload))
	binary.LittleEndian.PutUint32(frame, uint32(len(payload)))
	c.wmu.Lock()
	defer c.wmu.Unlock()
	_, err := c.rw.Write(append(frame, payload...))
	return err
}

// read hands each frame to its caller, or to Events, until the connection
// fails. It alone sends on events, so it alone closes them.
func (c *Client) read() {
	defer close(c.events)
	for {
		payload, err := readFrame(c.rw)
		if err != nil {
			c.fail(err)
			return
		}
		head, body, _ := strings.Cut(string(payload), "\n")
		idText, rest, _ := strings.Cut(head, " ")
		id, err := strconv.ParseUint(idText, 10, 64)
		if err != nil {
			c.fail(fmt.Errorf("the bridge sent a frame without an id: %q", head))
			return
		}
		if id == 0 {
			name, args, _ := strings.Cut(rest, " ")
			if name == "err" { // the reply to a request whose id the plugin couldn't read: no caller to give it to
				c.fail(fmt.Errorf("the bridge could not read a request: %s", args))
				return
			}
			select {
			case c.events <- Event{name, args}:
			default:
			}
			continue
		}
		c.mu.Lock()
		ch := c.pending[id]
		delete(c.pending, id)
		c.mu.Unlock()
		if ch == nil {
			c.fail(fmt.Errorf("the bridge replied to request %d, which was never sent", id))
			return
		}
		ch <- parseReply(rest, []byte(body))
	}
}

// parseReply reads a reply header after its id: "ok [summary]" or
// "err <code> <message>".
func parseReply(rest string, body []byte) result {
	if s, ok := strings.CutPrefix(rest, "ok"); ok && (s == "" || s[0] == ' ') {
		return result{r: Reply{Summary: strings.TrimPrefix(s, " "), Body: body}}
	}
	if s, ok := strings.CutPrefix(rest, "err "); ok {
		code, msg, _ := strings.Cut(s, " ")
		return result{err: &Error{Code: code, Message: msg}}
	}
	return result{err: fmt.Errorf("the bridge sent a reply that is neither ok nor err: %q", rest)}
}

func readFrame(r io.Reader) ([]byte, error) {
	var n [4]byte
	if _, err := io.ReadFull(r, n[:]); err != nil {
		return nil, err
	}
	size := binary.LittleEndian.Uint32(n[:])
	if size > maxFrame {
		return nil, fmt.Errorf("the bridge sent a %d-byte frame, more than %d", size, maxFrame)
	}
	b := make([]byte, size)
	_, err := io.ReadFull(r, b)
	return b, err
}

// fail ends the connection once: every pending call gets err.
func (c *Client) fail(err error) {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.err != nil {
		return
	}
	if errors.Is(err, io.EOF) || errors.Is(err, io.ErrUnexpectedEOF) {
		err = errors.New("the connection to Radiant ended: Radiant closed, or crashed")
	}
	c.err = err
	_ = c.rw.Close()
	for id, ch := range c.pending {
		ch <- result{err: err}
		delete(c.pending, id)
	}
}

func (c *Client) gone() error {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.err
}
