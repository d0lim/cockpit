package sqlport

import (
	"context"
	"database/sql/driver"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var portMutations = []porttest.Mutation{
	{"only declared databases are reachable", "fall back to the first database when the name is unknown", "red: an undeclared name returns rows", "TestUndeclaredDatabaseIsPortDeniedAndDeclaredDatabaseReturnsRows"},
	{"row limit", "remove the row count check in read", "red: three rows come back with truncated false", "TestRowsOverTheLimitAreTruncatedAndRowsWithinItAreNot"},
	{"byte limit", "remove the size check in read", "red: every row comes back with truncated false", "TestBytesOverTheLimitAreTruncatedAndBytesWithinItAreNot"},
	{"provisioning is serialized per database", "drop the entry mutex in ensure", "red: concurrent first queries provision more than once", "TestProvisioningRunsOnceForConcurrentQueriesAndAgainWithANewPasswordForANewPort"},
	{"each provisioning uses a fresh random password", "make newPassword return a constant", "red: two ports provision with the same password", "TestProvisioningRunsOnceForConcurrentQueriesAndAgainWithANewPasswordForANewPort"},
	{"queries run in a read-only transaction that is rolled back", "use BeginTx without ReadOnly or commit instead of rollback", "red: ReadOnly is false or a commit is recorded", "TestQueriesRunInAReadOnlyTransactionThatIsRolledBack"},
	{"the reader password and root secret never leave in errors", "return the backend error without scrub", "red: the planted password and root secret show in the error", "TestProvisionedPasswordAndRootSecretAreMaskedInErrors"},
	{"auth failure reprovisions once", "remove the authFailure retry", "red: an auth failure returns an error instead of rows", "TestAuthFailureReprovisionsOnceAndOtherFailuresDoNot"},
	{"missing root secret stops before provisioning", "use an empty root password when the secret is missing", "red: provisioning runs without a root secret", "TestMissingRootSecretIsPortUnavailableWithoutProvisioning"},
	{"cells pass the masker and binary cells are summarized", "return []byte cells as strings without mask", "red: the planted value or raw bytes come back", "TestRowStringsPassTheMaskerAndBinaryCellsAreSummarized"},
	{"time limit", "remove the WithTimeout around the query", "red: the blocked query never returns", "TestExceedingTheTimeLimitIsPortTimeoutAndQueriesWithinItPass"},
	{"a target is required for every database", "skip databases without a target", "red: New accepts a database without a target", "TestNewRejectsDatabasesWithoutATargetOrWithUnsafeIdentifiers"},
	{"schema and owner names are plain identifiers", "remove checkIdentifiers from validate", "red: a schema with a backtick is accepted", "TestNewRejectsDatabasesWithoutATargetOrWithUnsafeIdentifiers"},
}

func TestUndeclaredDatabaseIsPortDeniedAndDeclaredDatabaseReturnsRows(t *testing.T) {
	p, fb := fakePort(t, itemRows(2), Options{})
	_, err := p.Query(context.Background(), Request{DB: "other", Statement: "SELECT 1"})
	if codeOf(err) != engine.PortDenied || !strings.Contains(err.Error(), `"other" is not declared`) {
		t.Fatalf("%v", err)
	}
	if fb.provisions.Load() != 0 {
		t.Fatal("a denied query provisioned")
	}
	res, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT id, name FROM app.items"})
	if err != nil || len(res.Rows) != 2 || res.Truncated || res.Columns[1] != "name" || res.Rows[1][0] != int64(2) || res.Rows[1][1] != "item" {
		t.Fatalf("%+v %v", res, err)
	}
}

func TestRowsOverTheLimitAreTruncatedAndRowsWithinItAreNot(t *testing.T) {
	p, _ := fakePort(t, itemRows(3), Options{MaxRows: 3})
	res, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT * FROM app.items", MaxRows: 2})
	if err != nil || len(res.Rows) != 2 || !res.Truncated {
		t.Fatalf("over the limit: %d rows truncated=%v %v", len(res.Rows), res.Truncated, err)
	}
	res, err = p.Query(context.Background(), Request{DB: "main", Statement: "SELECT * FROM app.items"})
	if err != nil || len(res.Rows) != 3 || res.Truncated {
		t.Fatalf("at the limit: %d rows truncated=%v %v", len(res.Rows), res.Truncated, err)
	}
	res, err = p.Query(context.Background(), Request{DB: "main", Statement: "SELECT * FROM app.items", MaxRows: 50})
	if err != nil || len(res.Rows) != 3 || res.Truncated {
		t.Fatalf("a request above the port cap uses the cap: %d rows truncated=%v %v", len(res.Rows), res.Truncated, err)
	}
}

func TestBytesOverTheLimitAreTruncatedAndBytesWithinItAreNot(t *testing.T) {
	state := &fakeState{columns: []string{"v"}, types: []string{"TEXT"}}
	for range 4 {
		state.rows = append(state.rows, []driver.Value{strings.Repeat("x", 8)})
	}
	p, _ := fakePort(t, state, Options{})
	res, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT v FROM app.t", MaxBytes: 39})
	if err != nil || len(res.Rows) != 3 || !res.Truncated {
		t.Fatalf("over the limit: %d rows truncated=%v %v", len(res.Rows), res.Truncated, err)
	}
	res, err = p.Query(context.Background(), Request{DB: "main", Statement: "SELECT v FROM app.t", MaxBytes: 40})
	if err != nil || len(res.Rows) != 4 || res.Truncated {
		t.Fatalf("at the limit: %d rows truncated=%v %v", len(res.Rows), res.Truncated, err)
	}
}

func TestProvisioningRunsOnceForConcurrentQueriesAndAgainWithANewPasswordForANewPort(t *testing.T) {
	p, fb := fakePort(t, itemRows(1), Options{})
	fb.slow = 20 * time.Millisecond
	var wg sync.WaitGroup
	errs := make(chan error, 8)
	for range 8 {
		wg.Go(func() {
			_, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"})
			errs <- err
		})
	}
	wg.Wait()
	close(errs)
	ran := 0
	for err := range errs {
		ran++
		if err != nil {
			t.Fatal(err)
		}
	}
	if ran != 8 || fb.provisions.Load() != 1 {
		t.Fatalf("%d queries, %d provisions", ran, fb.provisions.Load())
	}
	var fresh []string
	for range 2 {
		q, fbq := fakePort(t, itemRows(1), Options{})
		q.password = newPassword
		if _, err := q.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"}); err != nil {
			t.Fatal(err)
		}
		if fbq.provisions.Load() != 1 || len(fbq.passwords) != 1 || len(fbq.passwords[0]) < 32 {
			t.Fatalf("new port passwords: %v", fbq.passwords)
		}
		fresh = append(fresh, fbq.passwords[0])
	}
	if fresh[0] == fresh[1] {
		t.Fatal("two ports provisioned with the same password")
	}
}

func TestQueriesRunInAReadOnlyTransactionThatIsRolledBack(t *testing.T) {
	state := itemRows(1)
	p, _ := fakePort(t, state, Options{})
	if _, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT id FROM app.items;  "}); err != nil {
		t.Fatal(err)
	}
	if len(state.readOnly) != 1 || !state.readOnly[0] || state.rollbacks != 1 || state.commits != 0 {
		t.Fatalf("readOnly=%v rollbacks=%d commits=%d", state.readOnly, state.rollbacks, state.commits)
	}
	if len(state.statements) != 1 || state.statements[0] != "SELECT id FROM app.items" {
		t.Fatalf("sent %q", state.statements)
	}
}

func TestProvisionedPasswordAndRootSecretAreMaskedInErrors(t *testing.T) {
	state := itemRows(1)
	p, fb := fakePort(t, state, Options{})
	fb.provisionErr = errorString("grant failed for PLANTED-run")
	_, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"})
	if err == nil || strings.Contains(err.Error(), planted) || strings.Contains(err.Error(), plantedPass) || strings.Contains(err.Error(), "PLANTED-run") || strings.Count(err.Error(), "****") != 3 {
		t.Fatalf("provision error: %v", err)
	}
	fb.provisionErr = nil
	state.queryErr = func(int) error { return errorString("server said " + plantedPass + " and " + planted) }
	_, err = p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"})
	if err == nil || strings.Contains(err.Error(), planted) || strings.Contains(err.Error(), plantedPass) || !strings.Contains(err.Error(), "server said **** and ****") {
		t.Fatalf("query error: %v", err)
	}
}

func TestAuthFailureReprovisionsOnceAndOtherFailuresDoNot(t *testing.T) {
	state := itemRows(1)
	state.queryErr = func(attempt int) error {
		if attempt == 0 {
			return errFakeAuth
		}
		return nil
	}
	p, fb := fakePort(t, state, Options{})
	res, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"})
	if err != nil || len(res.Rows) != 1 || fb.provisions.Load() != 2 {
		t.Fatalf("%+v %v provisions=%d", res, err, fb.provisions.Load())
	}
	state.queryErr = func(int) error { return errFakeAuth }
	if _, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"}); err == nil || fb.provisions.Load() != 3 {
		t.Fatalf("a second auth failure is returned after one retry: %v provisions=%d", err, fb.provisions.Load())
	}
	state.queryErr = func(int) error { return errorString("syntax") }
	if _, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"}); err == nil || fb.provisions.Load() != 3 {
		t.Fatalf("other failures do not reprovision: %v provisions=%d", err, fb.provisions.Load())
	}
}

func TestMissingRootSecretIsPortUnavailableWithoutProvisioning(t *testing.T) {
	p, fb := fakePort(t, itemRows(1), Options{Secret: func(string) (string, bool) { return "", false }})
	_, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"})
	if codeOf(err) != engine.PortUnavailable || !strings.Contains(err.Error(), `root secret "ROOT_PASSWORD"`) || fb.provisions.Load() != 0 {
		t.Fatalf("%v provisions=%d", err, fb.provisions.Load())
	}
	q, fb2 := fakePort(t, itemRows(1), Options{})
	if _, err := q.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1"}); err != nil || fb2.provisions.Load() != 1 {
		t.Fatalf("with the secret: %v provisions=%d", err, fb2.provisions.Load())
	}
}

func TestRowStringsPassTheMaskerAndBinaryCellsAreSummarized(t *testing.T) {
	when := time.Date(2026, 1, 2, 3, 4, 5, 0, time.FixedZone("x", 3600))
	state := &fakeState{columns: []string{"note", "blob", "raw", "at", "none", "n"}, types: []string{"VARCHAR", "BLOB", "TEXT", "DATETIME", "TEXT", "INT"}}
	state.rows = [][]driver.Value{{[]byte("token PLANTED-run end"), []byte("abc"), []byte{0xff, 0xfe}, when, nil, int64(7)}}
	p, _ := fakePort(t, state, Options{})
	res, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT * FROM app.t"})
	if err != nil {
		t.Fatal(err)
	}
	row := res.Rows[0]
	if row[0] != "token **** end" || row[1] != "<3 bytes>" || row[2] != "<2 bytes>" || row[3] != "2026-01-02T02:04:05Z" || row[4] != nil || row[5] != int64(7) {
		t.Fatalf("%#v", row)
	}
}

func TestExceedingTheTimeLimitIsPortTimeoutAndQueriesWithinItPass(t *testing.T) {
	state := itemRows(1)
	state.block = true
	p, _ := fakePort(t, state, Options{Timeout: time.Second})
	_, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1", TimeoutMs: 30})
	if codeOf(err) != engine.PortTimeout || !strings.Contains(err.Error(), "30ms") {
		t.Fatalf("%v", err)
	}
	state.mu.Lock()
	state.block = false
	state.mu.Unlock()
	if _, err := p.Query(context.Background(), Request{DB: "main", Statement: "SELECT 1", TimeoutMs: 500}); err != nil {
		t.Fatal(err)
	}
}

func TestNewRejectsDatabasesWithoutATargetOrWithUnsafeIdentifiers(t *testing.T) {
	good := contract.Database{Name: "main", Driver: contract.DriverPostgres, ReadSchemas: []string{"app"}, OwnerRole: "app_owner", RootSecret: "R"}
	target := map[string]Target{"main": {Host: "127.0.0.1", Port: 5432}}
	bad := []Options{
		{Databases: []contract.Database{good}},
		{Databases: []contract.Database{{Name: "main", Driver: contract.DriverMySQL, ReadSchemas: []string{"app`; DROP"}, RootSecret: "R"}}, Targets: target},
		{Databases: []contract.Database{{Name: "main", Driver: contract.DriverPostgres, ReadSchemas: []string{"app"}, OwnerRole: `o"x`, RootSecret: "R"}}, Targets: target},
		{Databases: []contract.Database{{Name: "main", Driver: "sqlite", ReadSchemas: []string{"app"}, RootSecret: "R"}}, Targets: target},
		{Databases: []contract.Database{{Name: "main", Driver: contract.DriverMySQL, ReadSchemas: nil, RootSecret: "R"}}, Targets: target},
	}
	for i, o := range bad {
		if _, err := New(o); err == nil {
			t.Fatalf("case %d accepted", i)
		}
	}
	if len(bad) != 5 {
		t.Fatalf("ran %d cases", len(bad))
	}
	p, err := New(Options{Databases: []contract.Database{good}, Targets: target})
	if err != nil {
		t.Fatal(err)
	}
	if pg := p.dbs["main"].backend.(pgBackend); pg.target.RootUser != "postgres" || pg.target.Database != "postgres" {
		t.Fatalf("defaults: %+v", pg.target)
	}
}

func TestPortMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "port_test.go", portMutations)
}

type errorString string

func (e errorString) Error() string { return string(e) }
