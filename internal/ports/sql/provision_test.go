package sqlport

import (
	"errors"
	"fmt"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/jackc/pgx/v5/pgconn"

	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var provisionMutations = []porttest.Mutation{
	{"MySQL grants SELECT on each read schema only", "grant SELECT ON *.* or ALL PRIVILEGES", "red: the statement list differs from the expected grants", "TestMySQLProvisioningCreatesOrResetsTheReaderAndGrantsSelectOnEachReadSchemaOnly"},
	{"PostgreSQL grants usage, select and owner defaults without pg_read_all_data", "grant pg_read_all_data instead of per-schema grants", "red: the statement list differs and contains pg_read_all_data", "TestPostgresProvisioningGrantsUsageSelectAndOwnerDefaultsWithoutPgReadAllData"},
	{"MySQL sessions are read-only with server limits", "drop transaction_read_only or max_execution_time from Params", "red: the reader config lacks the session parameter", "TestMySQLReaderSessionCarriesReadOnlyTimeLimitAndSelectLimit"},
	{"PostgreSQL sessions are read-only with a server time limit and ignore PG variables", "keep RuntimeParams from ParseConfig or skip clearPGEnv", "red: PGOPTIONS or PGPASSWORD reach the config or stay in the environment", "TestPostgresReaderSessionIsReadOnlyWithATimeLimitAndIgnoresPGEnvironment"},
	{"server error codes map to port codes", "map every server error to PORT_UNAVAILABLE", "red: 1142 and 42501 are not PORT_DENIED", "TestServerErrorCodesMapToPortCodesAndNameTheServerCode"},
}

func TestMySQLProvisioningCreatesOrResetsTheReaderAndGrantsSelectOnEachReadSchemaOnly(t *testing.T) {
	got := mysqlProvision([]string{"app", "audit"}, "pw")
	want := []string{
		"CREATE USER IF NOT EXISTS ?@'%' IDENTIFIED BY ? [cockpit_ro pw]",
		"ALTER USER ?@'%' IDENTIFIED BY ? [cockpit_ro pw]",
		"GRANT SELECT ON `app`.* TO ?@'%' [cockpit_ro]",
		"GRANT SELECT ON `audit`.* TO ?@'%' [cockpit_ro]",
	}
	if len(got) != len(want) || len(want) != 4 {
		t.Fatalf("%d statements", len(got))
	}
	for i := range want {
		if line := fmt.Sprintf("%s %v", got[i].text, got[i].args); line != want[i] {
			t.Errorf("statement %d: %s", i, line)
		}
	}
}

func TestPostgresProvisioningGrantsUsageSelectAndOwnerDefaultsWithoutPgReadAllData(t *testing.T) {
	got := pgProvision("appdb", []string{"app"}, "app_owner", "p'w")
	want := []string{
		`DO $cockpit$ BEGIN IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = 'cockpit_ro') THEN CREATE ROLE "cockpit_ro" LOGIN; END IF; END $cockpit$`,
		`ALTER ROLE "cockpit_ro" WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD 'p''w'`,
		`GRANT CONNECT ON DATABASE "appdb" TO "cockpit_ro"`,
		`GRANT USAGE ON SCHEMA "app" TO "cockpit_ro"`,
		`GRANT SELECT ON ALL TABLES IN SCHEMA "app" TO "cockpit_ro"`,
		`ALTER DEFAULT PRIVILEGES FOR ROLE "app_owner" IN SCHEMA "app" GRANT SELECT ON TABLES TO "cockpit_ro"`,
	}
	if len(got) != len(want) || len(want) != 6 {
		t.Fatalf("%d statements", len(got))
	}
	for i := range want {
		if got[i].text != want[i] || strings.Contains(got[i].text, "pg_read_all_data") {
			t.Errorf("statement %d: %s", i, got[i].text)
		}
	}
	if n := len(pgProvision("appdb", []string{"app"}, "", "pw")); n != 5 {
		t.Fatalf("without an owner role: %d statements", n)
	}
}

func TestMySQLReaderSessionCarriesReadOnlyTimeLimitAndSelectLimit(t *testing.T) {
	b := mysqlBackend{target: Target{Host: "127.0.0.1", Port: 23306}, max: limits{rows: 100, timeout: 2 * time.Second}}
	cfg := b.readerConfig("pw")
	if cfg.User != ReaderUser || cfg.Addr != "127.0.0.1:23306" || cfg.MultiStatements || cfg.AllowAllFiles || cfg.InterpolateParams {
		t.Fatalf("%+v", cfg)
	}
	want := map[string]string{"transaction_read_only": "1", "max_execution_time": "2000", "sql_select_limit": "101"}
	if len(cfg.Params) != len(want) {
		t.Fatalf("%v", cfg.Params)
	}
	for k, v := range want {
		if cfg.Params[k] != v {
			t.Errorf("%s = %q", k, cfg.Params[k])
		}
	}
	if root := b.config("root", "x"); len(root.Params) != 0 {
		t.Fatalf("the root session carries reader params: %v", root.Params)
	}
}

func TestPostgresReaderSessionIsReadOnlyWithATimeLimitAndIgnoresPGEnvironment(t *testing.T) {
	t.Setenv("PGOPTIONS", "-c default_transaction_read_only=off")
	t.Setenv("PGPASSWORD", planted)
	t.Setenv("PGDATABASE", "elsewhere")
	t.Setenv("PGAPPNAME", "other")
	t.Setenv("APP_PGLIKE", "kept")
	b := pgBackend{target: Target{Host: "127.0.0.1", Port: 25432, Database: "appdb"}, max: limits{timeout: 1500 * time.Millisecond}}
	cfg, err := b.config(ReaderUser, "pw", b.readerParams())
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]string{"default_transaction_read_only": "on", "statement_timeout": "1500", "application_name": "cockpit"}
	if len(cfg.RuntimeParams) != len(want) || cfg.Password != "pw" || cfg.Database != "appdb" || cfg.Port != 25432 || cfg.TLSConfig != nil || len(cfg.Fallbacks) != 0 {
		t.Fatalf("%+v", cfg.Config)
	}
	for k, v := range want {
		if cfg.RuntimeParams[k] != v {
			t.Errorf("%s = %q", k, cfg.RuntimeParams[k])
		}
	}
	clearPGEnv()
	for _, name := range []string{"PGOPTIONS", "PGPASSWORD", "PGDATABASE", "PGAPPNAME"} {
		if _, ok := os.LookupEnv(name); ok {
			t.Errorf("%s is still set", name)
		}
	}
	if os.Getenv("APP_PGLIKE") != "kept" {
		t.Fatal("a variable that does not start with PG was cleared")
	}
}

func TestServerErrorCodesMapToPortCodesAndNameTheServerCode(t *testing.T) {
	my, pg := mysqlBackend{}, pgBackend{}
	cases := []struct {
		code engine.PortCode
		got  func() (engine.PortCode, string)
		text string
	}{
		{engine.PortDenied, func() (engine.PortCode, string) {
			return my.classify(&mysql.MySQLError{Number: 1142, Message: "INSERT command denied"})
		}, "mysql 1142"},
		{engine.PortDenied, func() (engine.PortCode, string) {
			return my.classify(&mysql.MySQLError{Number: 1792, Message: "read only"})
		}, "mysql 1792"},
		{engine.PortTimeout, func() (engine.PortCode, string) {
			return my.classify(&mysql.MySQLError{Number: 3024, Message: "interrupted"})
		}, "mysql 3024"},
		{engine.PortBadArgs, func() (engine.PortCode, string) {
			return my.classify(&mysql.MySQLError{Number: 1064, Message: "syntax"})
		}, "mysql 1064"},
		{engine.PortUnavailable, func() (engine.PortCode, string) { return my.classify(errors.New("dial tcp: refused")) }, "failed to reach"},
		{engine.PortDenied, func() (engine.PortCode, string) {
			return pg.classify(&pgconn.PgError{Code: "42501", Message: "permission denied"})
		}, "postgres 42501"},
		{engine.PortDenied, func() (engine.PortCode, string) {
			return pg.classify(&pgconn.PgError{Code: "25006", Message: "read-only"})
		}, "postgres 25006"},
		{engine.PortTimeout, func() (engine.PortCode, string) {
			return pg.classify(&pgconn.PgError{Code: "57014", Message: "canceled"})
		}, "postgres 57014"},
		{engine.PortBadArgs, func() (engine.PortCode, string) {
			return pg.classify(&pgconn.PgError{Code: "42601", Message: "syntax"})
		}, "postgres 42601"},
		{engine.PortUnavailable, func() (engine.PortCode, string) { return pg.classify(errors.New("conn reset")) }, "failed to reach"},
	}
	ran := 0
	for _, c := range cases {
		code, text := c.got()
		if code != c.code || !strings.Contains(text, c.text) {
			t.Errorf("want %s with %q, got %s %q", c.code, c.text, code, text)
		}
		ran++
	}
	if ran != 10 {
		t.Fatalf("ran %d cases", ran)
	}
	if !my.authFailure(&mysql.MySQLError{Number: 1045}) || my.authFailure(&mysql.MySQLError{Number: 1142}) || !pg.authFailure(&pgconn.PgError{Code: "28P01"}) || pg.authFailure(&pgconn.PgError{Code: "42501"}) {
		t.Fatal("auth failure detection")
	}
}

func TestProvisionMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "provision_test.go", provisionMutations)
}
