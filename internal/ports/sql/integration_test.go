package sqlport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"
	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var integrationMutations = []porttest.Mutation{
	{"MySQL grants reach only the read schemas", "grant SELECT ON *.* in mysqlProvision", "red: the other schema is readable and the raw INSERT error is not 1142", "TestMySQLReaderReadsDeclaredSchemasAndRefusesWritesOtherSchemasAndFileExport"},
	{"MySQL sessions are read-only with server limits", "drop the reader Params", "red: transaction_read_only reads 0", "TestMySQLReaderReadsDeclaredSchemasAndRefusesWritesOtherSchemasAndFileExport"},
	{"PostgreSQL grants reach only the read schemas and owner tables", "drop ALTER DEFAULT PRIVILEGES or grant pg_read_all_data", "red: the later owner table is unreadable or the other schema is readable", "TestPostgresReaderReadsDeclaredSchemasAndRefusesWritesOtherSchemasAndLaterOwnerTablesAreRead"},
	{"PostgreSQL sessions are read-only with a server time limit", "drop the reader RuntimeParams and the ReadOnly transaction", "red: the data-modifying WITH is not denied with 25006", "TestPostgresReaderReadsDeclaredSchemasAndRefusesWritesOtherSchemasAndLaterOwnerTablesAreRead"},
}

type fixture struct {
	driver contract.DBDriver
	target Target
	admin  *sql.DB
	raw    func(password string) *sql.DB
	denied string
	later  []string
}

func integration(t *testing.T) context.Context {
	t.Helper()
	if os.Getenv("COCKPIT_INTEGRATION") != "1" {
		t.Skip("set COCKPIT_INTEGRATION=1 to run database integration tests")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Minute)
	t.Cleanup(cancel)
	return ctx
}

func endpoint(ctx context.Context, t *testing.T, c testcontainers.Container, port string) (string, int) {
	t.Helper()
	host, err := c.Host(ctx)
	if err != nil {
		t.Fatal(err)
	}
	mapped, err := c.MappedPort(ctx, port)
	if err != nil {
		t.Fatal(err)
	}
	return host, int(mapped.Num())
}

func mustExec(ctx context.Context, t *testing.T, db *sql.DB, stmts ...string) {
	t.Helper()
	for _, s := range stmts {
		if _, err := db.ExecContext(ctx, s); err != nil {
			t.Fatalf("%s: %v", s, err)
		}
	}
}

func TestMySQLReaderReadsDeclaredSchemasAndRefusesWritesOtherSchemasAndFileExport(t *testing.T) {
	ctx := integration(t)
	c, err := tcmysql.Run(ctx, "mysql:8.4", tcmysql.WithUsername("root"), tcmysql.WithPassword(planted), tcmysql.WithDatabase("app"))
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}
	host, port := endpoint(ctx, t, c, "3306/tcp")
	b := mysqlBackend{target: Target{Host: host, Port: port, RootUser: "root"}}
	admin := b.connect(b.config("root", planted))
	t.Cleanup(func() { _ = admin.Close() })
	mustExec(ctx, t, admin,
		"CREATE TABLE app.items (id INT PRIMARY KEY, name VARCHAR(32))",
		"INSERT INTO app.items VALUES (1, 'a'), (2, 'b'), (3, 'c')",
		"CREATE DATABASE other",
		"CREATE TABLE other.hidden (id INT)",
	)
	f := fixture{driver: contract.DriverMySQL, target: b.target, admin: admin, denied: "mysql 1142",
		raw:   func(pw string) *sql.DB { return b.connect(b.config(ReaderUser, pw)) },
		later: []string{"CREATE TABLE app.later (id INT)", "INSERT INTO app.later VALUES (7)"}}
	p, password := exercise(ctx, t, f)
	res, err := p.Query(ctx, Request{DB: "main", Statement: "SELECT @@session.transaction_read_only, @@session.max_execution_time, @@session.sql_select_limit"})
	if err != nil || len(res.Rows) != 1 || fmt.Sprint(res.Rows[0]...) != fmt.Sprint(1, defaultTimeout.Milliseconds(), defaultMaxRows+1) {
		t.Fatalf("session: %+v %v", res, err)
	}
	if _, err := p.Query(ctx, Request{DB: "main", Statement: "SELECT * FROM app.items INTO OUTFILE '/tmp/out'"}); codeOf(err) != engine.PortDenied {
		t.Fatalf("into outfile: %v", err)
	}
	raw := f.raw(password)
	defer raw.Close()
	var me *mysql.MySQLError
	if _, err := raw.ExecContext(ctx, "INSERT INTO app.items VALUES (9, 'z')"); !errors.As(err, &me) || me.Number != 1142 {
		t.Fatalf("raw insert: %v", err)
	}
	if _, err := raw.ExecContext(ctx, "SELECT * FROM app.items INTO OUTFILE '/tmp/out'"); !errors.As(err, &me) || me.Number != 1227 {
		t.Fatalf("raw into outfile: %v", err)
	}
}

func TestPostgresReaderReadsDeclaredSchemasAndRefusesWritesOtherSchemasAndLaterOwnerTablesAreRead(t *testing.T) {
	ctx := integration(t)
	c, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("appdb"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword(planted), tcpostgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}
	host, port := endpoint(ctx, t, c, "5432/tcp")
	b := pgBackend{target: Target{Host: host, Port: port, RootUser: "postgres", Database: "appdb"}}
	admin := b.connect("postgres", planted, nil)
	t.Cleanup(func() { _ = admin.Close() })
	mustExec(ctx, t, admin,
		"CREATE ROLE app_owner",
		"CREATE SCHEMA app AUTHORIZATION app_owner",
		"CREATE SCHEMA other",
		"CREATE TABLE other.hidden (id INT)",
		"SET ROLE app_owner",
		"CREATE TABLE app.items (id INT PRIMARY KEY, name TEXT)",
		"INSERT INTO app.items VALUES (1, 'a'), (2, 'b'), (3, 'c')",
		"RESET ROLE",
	)
	admin.SetMaxOpenConns(1)
	f := fixture{driver: contract.DriverPostgres, target: b.target, admin: admin, denied: "postgres 42501",
		raw: func(pw string) *sql.DB {
			cfg, _ := b.config(ReaderUser, pw, nil)
			return stdlib.OpenDB(*cfg)
		},
		later: []string{"SET ROLE app_owner", "CREATE TABLE app.later (id INT)", "INSERT INTO app.later VALUES (7)", "RESET ROLE"}}
	p, password := exercise(ctx, t, f)
	res, err := p.Query(ctx, Request{DB: "main", Statement: "SELECT current_setting('default_transaction_read_only'), current_setting('statement_timeout')"})
	if err != nil || len(res.Rows) != 1 || res.Rows[0][0] != "on" || res.Rows[0][1] != "10s" {
		t.Fatalf("session: %+v %v", res, err)
	}
	_, err = p.Query(ctx, Request{DB: "main", Statement: "WITH x AS (INSERT INTO app.items VALUES (9, 'z') RETURNING id) SELECT * FROM x"})
	if codeOf(err) != engine.PortDenied || !strings.Contains(err.Error(), "25006") {
		t.Fatalf("data-modifying with: %v", err)
	}
	raw := f.raw(password)
	defer raw.Close()
	var pe *pgconn.PgError
	if _, err := raw.ExecContext(ctx, "INSERT INTO app.items VALUES (9, 'z')"); !errors.As(err, &pe) || pe.Code != "42501" {
		t.Fatalf("raw insert: %v", err)
	}
}

func exercise(ctx context.Context, t *testing.T, f fixture) (*Port, string) {
	t.Helper()
	decl := contract.Database{Name: "main", Driver: f.driver, ReadSchemas: []string{"app"}, RootSecret: "ROOT_PASSWORD"}
	if f.driver == contract.DriverPostgres {
		decl.OwnerRole = "app_owner"
	}
	open := func() (*Port, string) {
		p, err := New(Options{Databases: []contract.Database{decl}, Targets: map[string]Target{"main": f.target},
			Secret: func(name string) (string, bool) { return planted, name == "ROOT_PASSWORD" }})
		if err != nil {
			t.Fatal(err)
		}
		var used string
		p.password = func() (string, error) {
			pw, err := newPassword()
			used = pw
			return pw, err
		}
		t.Cleanup(func() { _ = p.Close() })
		if _, err := p.Query(ctx, Request{DB: "main", Statement: "SELECT 1"}); err != nil {
			t.Fatalf("first query: %v", err)
		}
		return p, used
	}
	p, first := open()
	res, err := p.Query(ctx, Request{DB: "main", Statement: "SELECT name FROM app.items ORDER BY id"})
	if err != nil || len(res.Rows) != 3 || res.Rows[0][0] != "a" || res.Truncated {
		t.Fatalf("read: %+v %v", res, err)
	}
	if _, err := p.Query(ctx, Request{DB: "main", Statement: "INSERT INTO app.items VALUES (9, 'z')"}); codeOf(err) != engine.PortDenied {
		t.Fatalf("insert: %v", err)
	}
	_, err = p.Query(ctx, Request{DB: "main", Statement: "SELECT * FROM other.hidden"})
	if codeOf(err) != engine.PortDenied || !strings.Contains(err.Error(), f.denied) || strings.Contains(err.Error(), planted) {
		t.Fatalf("other schema: %v", err)
	}
	if _, err := p.Query(ctx, Request{DB: "main", Statement: "SELECT 1; SELECT 2"}); codeOf(err) != engine.PortDenied {
		t.Fatalf("two statements: %v", err)
	}
	mustExec(ctx, t, f.admin, f.later...)
	res, err = p.Query(ctx, Request{DB: "main", Statement: "SELECT id FROM app.later"})
	if err != nil || len(res.Rows) != 1 {
		t.Fatalf("later table: %+v %v", res, err)
	}
	if res, err := p.Query(ctx, Request{DB: "main", Statement: "SELECT id FROM app.items ORDER BY id", MaxRows: 2}); err != nil || len(res.Rows) != 2 || !res.Truncated {
		t.Fatalf("over the row limit: %+v %v", res, err)
	}
	if res, err := p.Query(ctx, Request{DB: "main", Statement: "SELECT id FROM app.items ORDER BY id", MaxRows: 3}); err != nil || len(res.Rows) != 3 || res.Truncated {
		t.Fatalf("at the row limit: %+v %v", res, err)
	}
	q, second := open()
	if second == first || second == "" {
		t.Fatal("the second provisioning reused the password")
	}
	if res, err := q.Query(ctx, Request{DB: "main", Statement: "SELECT name FROM app.items ORDER BY id"}); err != nil || len(res.Rows) != 3 {
		t.Fatalf("second pool after reprovisioning: %+v %v", res, err)
	}
	return q, second
}

func TestIntegrationMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "integration_test.go", integrationMutations)
}
