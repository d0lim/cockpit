package sqlport

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	tcmysql "github.com/testcontainers/testcontainers-go/modules/mysql"
	tcpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var paramIntegrationMutations = []porttest.Mutation{
	{"MySQL params are bound as data", "splice the params into the statement in run and pass no args", "red: the quoted param is a syntax error or the injected text runs", "TestMySQLParamsAreDataSoAQuoteAndADropStatementMatchNothingAndBreakNothing"},
	{"PostgreSQL params are bound as data", "splice the params into the statement in run and pass no args", "red: the quoted param is a syntax error or the injected text runs", "TestPostgresParamsAreDataSoAQuoteAndADropStatementMatchNothingAndBreakNothing"},
}

func TestParamIntegrationMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "integration_params_test.go", paramIntegrationMutations)
}

const injection = "x'; DROP TABLE items; --"

type paramCase struct {
	stmt   string
	params []any
	rows   int
}

func startMySQL(ctx context.Context, t *testing.T, seed ...string) Target {
	t.Helper()
	c, err := tcmysql.Run(ctx, "mysql:8.4", tcmysql.WithUsername("root"), tcmysql.WithPassword(planted), tcmysql.WithDatabase("app"))
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}
	host, port := endpoint(ctx, t, c, "3306/tcp")
	b := mysqlBackend{target: Target{Host: host, Port: port, RootUser: "root"}}
	admin := b.connect(b.config("root", planted))
	t.Cleanup(func() { _ = admin.Close() })
	mustExec(ctx, t, admin, seed...)
	return b.target
}

func startPostgres(ctx context.Context, t *testing.T, seed ...string) Target {
	t.Helper()
	c, err := tcpostgres.Run(ctx, "postgres:16-alpine", tcpostgres.WithDatabase("appdb"), tcpostgres.WithUsername("postgres"), tcpostgres.WithPassword(planted), tcpostgres.BasicWaitStrategies())
	testcontainers.CleanupContainer(t, c)
	if err != nil {
		t.Fatal(err)
	}
	host, port := endpoint(ctx, t, c, "5432/tcp")
	b := pgBackend{target: Target{Host: host, Port: port, RootUser: "postgres", Database: "appdb"}}
	admin := b.connect("postgres", planted, nil)
	t.Cleanup(func() { _ = admin.Close() })
	admin.SetMaxOpenConns(1)
	mustExec(ctx, t, admin, seed...)
	return b.target
}

func openPort(t *testing.T, decl contract.Database, target Target) *Port {
	t.Helper()
	p, err := New(Options{Databases: []contract.Database{decl}, Targets: map[string]Target{"main": target},
		Secret: func(name string) (string, bool) { return planted, name == "ROOT_PASSWORD" }})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = p.Close() })
	return p
}

func checkParamCases(ctx context.Context, t *testing.T, p *Port, cases []paramCase, mismatch paramCase) {
	t.Helper()
	ran := 0
	for _, c := range cases {
		res, err := p.Query(ctx, Request{DB: "main", Statement: c.stmt, Params: c.params})
		if err != nil || len(res.Rows) != c.rows {
			t.Errorf("%s %v: %+v %v", c.stmt, c.params, res, err)
		}
		ran++
	}
	if ran != len(cases) || ran == 0 {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
	if _, err := p.Query(ctx, Request{DB: "main", Statement: mismatch.stmt, Params: mismatch.params}); codeOf(err) != engine.PortBadArgs {
		t.Fatalf("params that do not match the placeholders: %v", err)
	}
}

func TestMySQLParamsAreDataSoAQuoteAndADropStatementMatchNothingAndBreakNothing(t *testing.T) {
	ctx := integration(t)
	target := startMySQL(ctx, t,
		"CREATE TABLE app.items (id INT PRIMARY KEY, name VARCHAR(64))",
		"INSERT INTO app.items VALUES (1, 'a'), (2, 'o''brien'), (3, 'x''; DROP TABLE items; --')",
	)
	p := openPort(t, contract.Database{Name: "main", Driver: contract.DriverMySQL, ReadSchemas: []string{"app"}, RootSecret: "ROOT_PASSWORD"}, target)
	checkParamCases(ctx, t, p, []paramCase{
		{"SELECT name FROM app.items WHERE name = ?", []any{"o'brien"}, 1},
		{"SELECT name FROM app.items WHERE name = ?", []any{"' OR '1'='1"}, 0},
		{"SELECT name FROM app.items WHERE name = ?", []any{injection}, 1},
		{"SELECT name FROM app.items WHERE id = ? AND name = ?", []any{int64(1), "a"}, 1},
		{"SELECT name FROM app.items WHERE id > ? AND ? AND ? IS NULL", []any{1.5, true, nil}, 2},
		{"SELECT COUNT(*) FROM app.items", nil, 1},
	}, paramCase{"SELECT name FROM app.items WHERE id = ? AND name = ?", []any{int64(1)}, 0})
	res, err := p.Query(ctx, Request{DB: "main", Statement: "SELECT COUNT(*) FROM app.items"})
	if err != nil || len(res.Rows) != 1 || res.Rows[0][0] != int64(3) {
		t.Fatalf("the table after the injection attempts: %+v %v", res, err)
	}
}

func TestPostgresParamsAreDataSoAQuoteAndADropStatementMatchNothingAndBreakNothing(t *testing.T) {
	ctx := integration(t)
	target := startPostgres(ctx, t,
		"CREATE SCHEMA app",
		"CREATE TABLE app.items (id INT PRIMARY KEY, name TEXT)",
		"INSERT INTO app.items VALUES (1, 'a'), (2, 'o''brien'), (3, 'x''; DROP TABLE items; --')",
	)
	p := openPort(t, contract.Database{Name: "main", Driver: contract.DriverPostgres, ReadSchemas: []string{"app"}, RootSecret: "ROOT_PASSWORD"}, target)
	checkParamCases(ctx, t, p, []paramCase{
		{"SELECT name FROM app.items WHERE name = $1", []any{"o'brien"}, 1},
		{"SELECT name FROM app.items WHERE name = $1", []any{"' OR '1'='1"}, 0},
		{"SELECT name FROM app.items WHERE name = $1", []any{injection}, 1},
		{"SELECT name FROM app.items WHERE id = $1 AND name = $2", []any{int64(1), "a"}, 1},
		{"SELECT name FROM app.items WHERE id > $1::float8 AND $2::boolean AND $3::text IS NULL", []any{1.5, true, nil}, 2},
		{"SELECT COUNT(*) FROM app.items", nil, 1},
	}, paramCase{"SELECT name FROM app.items WHERE id = $1 AND name = $2", []any{int64(1)}, 0})
	res, err := p.Query(ctx, Request{DB: "main", Statement: "SELECT COUNT(*) FROM app.items"})
	if err != nil || len(res.Rows) != 1 || res.Rows[0][0] != int64(3) {
		t.Fatalf("the table after the injection attempts: %+v %v", res, err)
	}
}
