package sqlport

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"log"
	"log/slog"
	"net"
	"os"
	"strconv"
	"strings"
	"sync"
	"testing"

	"github.com/go-sql-driver/mysql"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var driverLogMutations = []porttest.Mutation{
	{"the run's mysql connections log through the run logger", "drop the cfg.Logger assignment in mysqlBackend.config", "red: the sink has no driver record", "TestTheMySQLDriverLogsThroughTheRunLoggerAtWarnWithTheRunIDAndTheRunMask"},
	{"the run mask reaches the driver log", "New builds the driver log with a nil mask", "red: EOF stays in the sink", "TestTheMySQLDriverLogsThroughTheRunLoggerAtWarnWithTheRunIDAndTheRunMask"},
	{"driver log detail is masked", "Print writes the detail without calling the mask", "red: the planted value shows up in the sink", "TestDriverLogDetailPassesTheMaskSoAPlantedValueBecomesStars"},
	{"the driver's default logger is replaced", "RouteDriverLog returns nil without calling mysql.SetLogger", "red: the DSN connection's record is missing from the sink", "TestRouteDriverLogReplacesTheDriversDefaultLoggerForConnectionsOpenedFromADSN"},
}

func TestDriverLogMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "driverlog_test.go", driverLogMutations)
}

type syncBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *syncBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *syncBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func hangUpListener(t *testing.T) int {
	t.Helper()
	l, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { l.Close() })
	go func() {
		for {
			c, err := l.Accept()
			if err != nil {
				return
			}
			c.Close()
		}
	}()
	return l.Addr().(*net.TCPAddr).Port
}

func TestTheMySQLDriverLogsThroughTheRunLoggerAtWarnWithTheRunIDAndTheRunMask(t *testing.T) {
	var sink syncBuffer
	d := contract.Database{Name: "main", Driver: contract.DriverMySQL, Port: "db", ReadSchemas: []string{"app"}, RootSecret: "R"}
	p, err := New(Options{
		Databases: []contract.Database{d},
		Targets:   map[string]Target{"main": {Host: "127.0.0.1", Port: hangUpListener(t)}},
		Secret:    func(string) (string, bool) { return "root-value", true },
		Mask:      func(s string) string { return strings.ReplaceAll(s, "EOF", "****") },
		Log:       slog.New(slog.NewTextHandler(&sink, nil)).With("run", "run-7"),
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	if _, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"}); codeOf(err) != engine.PortUnavailable {
		t.Fatalf("a server that hangs up must be unavailable, got %v", err)
	}
	out := sink.String()
	for _, want := range []string{"level=WARN", "run=run-7", "the mysql driver reported a connection error", "packets.go:", "****"} {
		if !strings.Contains(out, want) {
			t.Errorf("the sink lacks %q: %q", want, out)
		}
	}
	if strings.Contains(out, "EOF") {
		t.Errorf("the run mask did not reach the driver log: %q", out)
	}
}

func TestDriverLogDetailPassesTheMaskSoAPlantedValueBecomesStars(t *testing.T) {
	var sink syncBuffer
	l := newDriverLog(slog.New(slog.NewTextHandler(&sink, nil)), func(s string) string { return strings.ReplaceAll(s, planted, "****") })
	l.Print("packets.go:155 ", errors.New("write tcp "+planted+": broken pipe"))
	out := sink.String()
	if strings.Contains(out, planted) || !strings.Contains(out, "****") || !strings.Contains(out, "packets.go:155 write tcp") || !strings.Contains(out, "broken pipe") {
		t.Fatalf("sink %q", out)
	}
}

func TestRouteDriverLogReplacesTheDriversDefaultLoggerForConnectionsOpenedFromADSN(t *testing.T) {
	var sink syncBuffer
	t.Cleanup(func() { _ = mysql.SetLogger(log.New(os.Stderr, "[mysql] ", log.Ldate|log.Ltime)) })
	if err := RouteDriverLog(slog.New(slog.NewTextHandler(&sink, nil))); err != nil {
		t.Fatal(err)
	}
	db, err := sql.Open("mysql", "reader:pw@tcp(127.0.0.1:"+strconv.Itoa(hangUpListener(t))+")/")
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	if err := db.PingContext(context.Background()); err == nil {
		t.Fatal("a server that hangs up must fail the ping")
	}
	out := sink.String()
	if !strings.Contains(out, "level=WARN") || !strings.Contains(out, "packets.go:") || !strings.Contains(out, "the mysql driver reported a connection error") {
		t.Fatalf("the default driver logger did not reach the sink: %q", out)
	}
}
