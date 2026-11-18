package recorder

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"

	"github.com/d0lim/cockpit/internal/wire"
)

const (
	controlRequestLimit = 5 * time.Second
	controlDialLimit    = time.Second
	maxControlRequest   = 256
	replyOK             = "ok"
	replyError          = "error "
)

func ControlAddress() string {
	return net.JoinHostPort("127.0.0.1", strconv.Itoa(ControlPort))
}

func (r *Recorder) serveControl(ctx context.Context) {
	for {
		conn, err := r.control.Accept()
		if err != nil {
			return
		}
		go r.handleControl(ctx, conn)
	}
}

func (r *Recorder) handleControl(ctx context.Context, conn net.Conn) {
	defer conn.Close()
	conn.SetReadDeadline(time.Now().Add(controlRequestLimit))
	text, err := bufio.NewReader(io.LimitReader(conn, maxControlRequest)).ReadString('\n')
	if err != nil {
		return
	}
	conn.SetReadDeadline(time.Time{})
	out := bufio.NewWriter(conn)
	defer out.Flush()
	words := strings.Fields(text)
	switch {
	case len(words) == 1 && words[0] == "ping":
		fmt.Fprintln(out, replyOK)
	case len(words) == 2 && words[0] == "arm" && ValidTake(words[1]):
		r.store.arm(words[1])
		r.log.Info("wire take armed", "take", words[1])
		fmt.Fprintln(out, replyOK)
	case len(words) == 2 && words[0] == "take" && ValidTake(words[1]):
		r.writeTake(ctx, out, words[1])
	default:
		fmt.Fprintln(out, replyError+"unknown control request")
	}
}

func (r *Recorder) writeTake(ctx context.Context, out *bufio.Writer, name string) {
	rec, err := r.store.takeRecord(ctx, name)
	if err != nil {
		fmt.Fprintln(out, replyError+err.Error())
		return
	}
	fmt.Fprintln(out, replyOK)
	for _, l := range rec.lines {
		out.Write(l)
		out.WriteByte('\n')
	}
	end, err := json.Marshal(wire.Line{End: &rec.end})
	if err != nil {
		return
	}
	out.Write(end)
	out.WriteByte('\n')
	r.log.Info("wire take written", "take", name, "calls", rec.end.Calls, "bodiesOmitted", rec.end.BodiesOmitted, "callsDropped", rec.end.CallsDropped)
}

func Control(ctx context.Context, addr, request string, stdout io.Writer) error {
	dial, cancel := context.WithTimeout(ctx, controlDialLimit)
	defer cancel()
	conn, err := (&net.Dialer{}).DialContext(dial, "tcp", addr)
	if err != nil {
		return errors.New("the wire recorder is not reachable")
	}
	defer conn.Close()
	stop := context.AfterFunc(ctx, func() { conn.Close() })
	defer stop()
	if _, err := io.WriteString(conn, request+"\n"); err != nil {
		return errors.New("the wire recorder did not take the request")
	}
	in := bufio.NewReader(conn)
	reply, err := in.ReadString('\n')
	if err != nil {
		return errors.New("the wire recorder closed the connection without an answer")
	}
	reply = strings.TrimSuffix(reply, "\n")
	if message, failed := strings.CutPrefix(reply, replyError); failed {
		return errors.New(message)
	}
	if reply != replyOK {
		return errors.New("the wire recorder gave an unknown answer")
	}
	if !strings.HasPrefix(request, "take ") {
		return nil
	}
	return copyTake(in, stdout)
}

func copyTake(in *bufio.Reader, stdout io.Writer) error {
	var last []byte
	for {
		text, err := in.ReadBytes('\n')
		if len(text) > 0 {
			if _, werr := stdout.Write(text); werr != nil {
				return errors.New("could not write the take to stdout")
			}
			last = text
		}
		if err == io.EOF {
			break
		}
		if err != nil {
			return errors.New("the wire take broke off")
		}
	}
	var final wire.Line
	if json.Unmarshal(last, &final) != nil || final.End == nil {
		return errors.New("the wire take ended without its end line")
	}
	return nil
}
