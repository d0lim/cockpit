package sqlport

import (
	"context"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var statementMutations = []porttest.Mutation{
	{"one statement only", "drop the ended check in scanner.run", "red: SELECT 1; DELETE ... is accepted", "TestStatementsThatAreNotASingleReadStatementArePortDeniedAndReadStatementsPass"},
	{"read verbs only", "remove the readVerbs check in verdict", "red: INSERT, UPDATE, SET and CALL are accepted", "TestStatementsThatAreNotASingleReadStatementArePortDeniedAndReadStatementsPass"},
	{"INTO OUTFILE is refused", "remove the into outfile check in verdict", "red: SELECT ... INTO OUTFILE is accepted", "TestStatementsThatAreNotASingleReadStatementArePortDeniedAndReadStatementsPass"},
	{"MySQL executable comments are refused", "treat /*! like any block comment", "red: a statement hidden in /*! */ is accepted", "TestStatementsThatAreNotASingleReadStatementArePortDeniedAndReadStatementsPass"},
	{"both backslash readings must agree", "scan only with backslash escapes on", "red: a quote escaped by a backslash hides a second statement", "TestStatementsThatAreNotASingleReadStatementArePortDeniedAndReadStatementsPass"},
	{"semicolons inside literals do not split", "stop skipping quoted literals in scanner.run", "red: SELECT ';' is denied", "TestStatementsThatAreNotASingleReadStatementArePortDeniedAndReadStatementsPass"},
	{"the statement text never appears in a refusal", "append the statement to the refusal reason", "red: the planted value shows in the error", "TestRefusalsNameTheReasonAndDatabaseButNotTheStatement"},
}

type statementCase struct {
	driver contract.DBDriver
	stmt   string
	want   engine.PortCode
}

func TestStatementsThatAreNotASingleReadStatementArePortDeniedAndReadStatementsPass(t *testing.T) {
	my, pg := contract.DriverMySQL, contract.DriverPostgres
	cases := []statementCase{
		{my, "SELECT id FROM app.items", ""},
		{my, "  with x as (select 1) select * from x", ""},
		{my, "SELECT ';' AS semi, \"a;b\", `c;d` FROM app.items", ""},
		{my, "SELECT 1 -- trailing; comment\n", ""},
		{my, "SELECT 1 # hash; comment", ""},
		{my, "SELECT 1 /* block; */", ""},
		{my, "SHOW TABLES FROM app", ""},
		{my, "EXPLAIN SELECT 1", ""},
		{my, "SELECT 1;", ""},
		{pg, "SELECT $$a;b$$, $tag$ ; $tag$, $1", ""},
		{pg, "SELECT 1 /* outer /* inner; */ still; */", ""},
		{pg, "TABLE app.items", ""},
		{pg, "VALUES (1)", ""},
		{my, "SELECT 1; DELETE FROM app.items", engine.PortDenied},
		{my, "SELECT 1;\n-- comment\nDROP TABLE app.items", engine.PortDenied},
		{my, "INSERT INTO app.items VALUES (1)", engine.PortDenied},
		{my, "update app.items set name = 'x'", engine.PortDenied},
		{my, "DELETE FROM app.items", engine.PortDenied},
		{my, "SET SESSION max_execution_time = 0", engine.PortDenied},
		{my, "CALL app.proc()", engine.PortDenied},
		{my, "LOAD DATA INFILE '/etc/passwd' INTO TABLE app.items", engine.PortDenied},
		{my, "SELECT * FROM app.items INTO OUTFILE '/tmp/out'", engine.PortDenied},
		{my, "SELECT * FROM app.items INTO\nDUMPFILE '/tmp/out'", engine.PortDenied},
		{my, "SELECT 1 /*!; DROP TABLE app.items */", engine.PortDenied},
		{my, `SELECT 'a\'; DROP TABLE app.items; -- '`, engine.PortDenied},
		{my, "SELECT 1--1; DROP TABLE app.items", engine.PortDenied},
		{pg, "SELECT 1; SET default_transaction_read_only = off", engine.PortDenied},
		{pg, "SET default_transaction_read_only = off", engine.PortDenied},
		{pg, "COPY app.items TO '/tmp/out'", engine.PortDenied},
		{my, "", engine.PortBadArgs},
		{my, "  -- only a comment", engine.PortBadArgs},
		{my, "SELECT 'unclosed", engine.PortBadArgs},
		{pg, "SELECT $x$ unclosed", engine.PortBadArgs},
		{pg, "SELECT 1 /* unclosed", engine.PortBadArgs},
		{pg, "SELECT 1 /* outer /* inner */ ; DROP TABLE x", engine.PortBadArgs},
	}
	p, _ := fakePort(t, itemRows(1), Options{})
	ran := 0
	for _, c := range cases {
		p.dbs["main"].decl.Driver = c.driver
		_, err := p.Query(context.Background(), Request{DB: "main", Statement: c.stmt})
		if codeOf(err) != c.want {
			t.Errorf("%s %q: got %v, want %q", c.driver, c.stmt, err, c.want)
		}
		ran++
	}
	if ran != 35 || ran != len(cases) {
		t.Fatalf("ran %d of %d cases", ran, len(cases))
	}
}

func TestRefusalsNameTheReasonAndDatabaseButNotTheStatement(t *testing.T) {
	p, _ := fakePort(t, itemRows(1), Options{})
	_, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT '" + planted + "'; DELETE FROM app.items"})
	if codeOf(err) != engine.PortDenied || strings.Contains(err.Error(), planted) || !strings.Contains(err.Error(), "only one statement is accepted — db main") {
		t.Fatalf("%v", err)
	}
	_, err = p.Query(context.Background(), Request{DB: "PLANTED-run-db", Statement: "SELECT 1"})
	if codeOf(err) != engine.PortDenied || strings.Contains(err.Error(), "PLANTED-run") || !strings.Contains(err.Error(), "****") {
		t.Fatalf("%v", err)
	}
}

func TestStatementMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "statement_test.go", statementMutations)
}
