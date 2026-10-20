package live

import (
	"context"
	"errors"
	"log/slog"
	"math"
	"net/http"
	"time"

	"github.com/gorilla/websocket"

	"github.com/d0lim/cockpit/internal/hub/sse"
	"github.com/d0lim/cockpit/internal/signal"
)

const (
	DefaultFlush = 250 * time.Millisecond
	DefaultPoll  = 500 * time.Millisecond
	DefaultPing  = 15 * time.Second
	writeWait    = 10 * time.Second
	signalBatch  = 500
	ScreenParam  = "screen"
)

var ErrGone = errors.New("the screen's run is gone")

type Screen struct {
	Run     string
	Regions []string
	Touches map[sse.Kind][]string
	Signals []string
	Render  func(ctx context.Context, regions []string) ([]byte, error)
}

type Resolver func(ctx context.Context, path string) (*Screen, error)

type Handler struct {
	Bus     *sse.Bus
	Signals sse.Signals
	Resolve Resolver
	Flush   time.Duration
	Poll    time.Duration
	Ping    time.Duration
	Log     *slog.Logger
}

var upgrader = websocket.Upgrader{CheckOrigin: func(*http.Request) bool { return true }}

func (h *Handler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	conn, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		return
	}
	defer conn.Close()
	ctx, cancel := context.WithCancel(r.Context())
	defer cancel()
	go func() {
		defer cancel()
		for {
			if _, _, err := conn.ReadMessage(); err != nil {
				return
			}
		}
	}()
	sc, err := h.Resolve(ctx, r.URL.Query().Get(ScreenParam))
	if err != nil || sc == nil {
		closeNormally(conn, "no live screen")
		return
	}
	c := &connection{h: h, conn: conn, sc: sc, dirty: map[string]bool{}}
	c.serve(ctx)
}

type connection struct {
	h        *Handler
	conn     *websocket.Conn
	sc       *Screen
	dirty    map[string]bool
	seen     int64
	polling  bool
	finished bool
}

func (c *connection) target() string {
	if c.sc.Run == "" {
		return sse.HubTarget
	}
	return c.sc.Run
}

func (c *connection) mark(regions []string) {
	for _, id := range regions {
		c.dirty[id] = true
	}
}

func (c *connection) serve(ctx context.Context) {
	_, events, unsubscribe := c.h.Bus.Subscribe(c.target(), math.MaxInt64)
	defer func() { unsubscribe() }()
	c.polling = c.h.Signals != nil && c.sc.Run != "" && len(c.sc.Signals) > 0
	if c.polling {
		c.pollSignals()
	}
	c.mark(c.sc.Regions)
	c.flush(ctx)
	flush := time.NewTicker(orDefault(c.h.Flush, DefaultFlush))
	defer flush.Stop()
	poll := time.NewTicker(orDefault(c.h.Poll, DefaultPoll))
	defer poll.Stop()
	ping := time.NewTicker(orDefault(c.h.Ping, DefaultPing))
	defer ping.Stop()
	for !c.finished {
		select {
		case <-ctx.Done():
			return
		case e, ok := <-events:
			if !ok {
				unsubscribe()
				_, events, unsubscribe = c.h.Bus.Subscribe(c.target(), math.MaxInt64)
				c.mark(c.sc.Regions)
				continue
			}
			c.mark(c.sc.Touches[e.Kind])
		case <-poll.C:
			if c.polling && c.pollSignals() {
				c.mark(c.sc.Signals)
			}
		case <-flush.C:
			c.flush(ctx)
		case <-ping.C:
			if c.conn.WriteControl(websocket.PingMessage, nil, time.Now().Add(writeWait)) != nil {
				return
			}
		}
	}
}

func (c *connection) flush(ctx context.Context) {
	if len(c.dirty) == 0 {
		return
	}
	var regions []string
	for _, id := range c.sc.Regions {
		if c.dirty[id] {
			regions = append(regions, id)
		}
	}
	clear(c.dirty)
	if len(regions) == 0 {
		return
	}
	body, err := c.sc.Render(ctx, regions)
	if errors.Is(err, ErrGone) {
		closeNormally(c.conn, "the run is gone")
		c.finished = true
		return
	}
	if err != nil {
		c.log().Warn("live render failed", "error", err)
		return
	}
	c.conn.SetWriteDeadline(time.Now().Add(writeWait))
	if c.conn.WriteMessage(websocket.TextMessage, body) != nil {
		c.finished = true
	}
}

func (c *connection) pollSignals() bool {
	changed := false
	for {
		resp, err := c.h.Signals.Read(c.sc.Run, signal.ReadRequest{After: c.seen, Limit: signalBatch})
		if err != nil {
			c.polling = false
			return true
		}
		if resp.Resync != nil || len(resp.Signals) > 0 {
			changed = true
		}
		c.seen = max(c.seen, resp.Next)
		if len(resp.Signals) < signalBatch {
			return changed
		}
	}
}

func (c *connection) log() *slog.Logger {
	if c.h.Log == nil {
		return slog.New(slog.DiscardHandler)
	}
	return c.h.Log
}

func closeNormally(conn *websocket.Conn, reason string) {
	msg := websocket.FormatCloseMessage(websocket.CloseNormalClosure, reason)
	conn.WriteControl(websocket.CloseMessage, msg, time.Now().Add(writeWait))
}

func orDefault(d, fallback time.Duration) time.Duration {
	if d <= 0 {
		return fallback
	}
	return d
}
