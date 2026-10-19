package main

import (
	"bufio"
	"errors"
	"fmt"
	"io"
	"net"
	"strconv"
	"strings"
	"time"
)

var errNil = errors.New("nil reply")

type cache struct {
	addr string
}

func (c cache) do(args ...string) (string, error) {
	conn, err := net.DialTimeout("tcp", c.addr, 3*time.Second)
	if err != nil {
		return "", err
	}
	defer conn.Close()
	conn.SetDeadline(time.Now().Add(5 * time.Second))
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := conn.Write([]byte(b.String())); err != nil {
		return "", err
	}
	return readReply(bufio.NewReader(conn))
}

func readReply(r *bufio.Reader) (string, error) {
	head, err := r.ReadString('\n')
	if err != nil {
		return "", err
	}
	head = strings.TrimRight(head, "\r\n")
	if head == "" {
		return "", errors.New("empty reply")
	}
	switch head[0] {
	case '+', ':':
		return head[1:], nil
	case '-':
		return "", errors.New(head[1:])
	case '$':
		n, err := strconv.Atoi(head[1:])
		if err != nil {
			return "", err
		}
		if n < 0 {
			return "", errNil
		}
		buf := make([]byte, n+2)
		if _, err := io.ReadFull(r, buf); err != nil {
			return "", err
		}
		return string(buf[:n]), nil
	}
	return "", fmt.Errorf("unsupported reply %q", head[:1])
}
