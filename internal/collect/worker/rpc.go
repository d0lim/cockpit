package worker

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"sync"

	"github.com/d0lim/cockpit/internal/engine"
)

const SubCommand = "script-host"

type Setup struct {
	Collector    string   `json:"collector"`
	Code         string   `json:"code"`
	Allowed      []string `json:"allowed"`
	DeadlineMs   int64    `json:"deadlineMs"`
	MaxCallStack int      `json:"maxCallStack,omitempty"`
}

type FrameError struct {
	Code    engine.PortCode `json:"code"`
	Message string          `json:"message"`
}

type Done struct {
	OK      bool   `json:"ok"`
	Kind    string `json:"kind,omitempty"`
	Message string `json:"message,omitempty"`
}

type Frame struct {
	ID     int64             `json:"id,omitempty"`
	Port   string            `json:"port,omitempty"`
	Method string            `json:"method,omitempty"`
	Args   []json.RawMessage `json:"args,omitempty"`
	Result json.RawMessage   `json:"result,omitempty"`
	Error  *FrameError       `json:"error,omitempty"`
	Done   *Done             `json:"done,omitempty"`
}

type writer struct {
	mu sync.Mutex
	w  io.Writer
}

func (w *writer) send(f Frame) error {
	line, err := json.Marshal(f)
	if err != nil {
		return err
	}
	w.mu.Lock()
	defer w.mu.Unlock()
	_, err = w.w.Write(append(line, '\n'))
	return err
}

var errClosed = errors.New("the hub closed the connection")

type client struct {
	out     *writer
	mu      sync.Mutex
	next    int64
	pending map[int64]chan Frame
	closed  bool
}

func newClient(out io.Writer) *client {
	return &client{out: &writer{w: out}, pending: map[int64]chan Frame{}}
}

func (c *client) serve(in *bufio.Reader) {
	for {
		line, err := in.ReadBytes('\n')
		if len(line) > 0 {
			var f Frame
			if json.Unmarshal(line, &f) == nil {
				c.mu.Lock()
				ch := c.pending[f.ID]
				delete(c.pending, f.ID)
				c.mu.Unlock()
				if ch != nil {
					ch <- f
				}
			}
		}
		if err != nil {
			c.mu.Lock()
			c.closed = true
			for id, ch := range c.pending {
				close(ch)
				delete(c.pending, id)
			}
			c.mu.Unlock()
			return
		}
	}
}

func (c *client) call(ctx context.Context, port, method string, args []json.RawMessage) (json.RawMessage, error) {
	ch := make(chan Frame, 1)
	c.mu.Lock()
	if c.closed {
		c.mu.Unlock()
		return nil, &engine.PortError{Code: engine.PortUnavailable, Message: errClosed.Error()}
	}
	c.next++
	id := c.next
	c.pending[id] = ch
	c.mu.Unlock()
	if err := c.out.send(Frame{ID: id, Port: port, Method: method, Args: args}); err != nil {
		return nil, &engine.PortError{Code: engine.PortUnavailable, Message: errClosed.Error()}
	}
	select {
	case f, ok := <-ch:
		if !ok {
			return nil, &engine.PortError{Code: engine.PortUnavailable, Message: errClosed.Error()}
		}
		if f.Error != nil {
			return nil, &engine.PortError{Code: f.Error.Code, Message: f.Error.Message}
		}
		return f.Result, nil
	case <-ctx.Done():
		return nil, &engine.PortError{Code: engine.PortTimeout, Message: fmt.Sprintf("%s.%s was canceled", port, method)}
	}
}
