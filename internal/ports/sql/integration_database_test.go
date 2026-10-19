package sqlport

import (
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var databaseIntegrationMutations = []porttest.Mutation{
	{"the declared MySQL database resolves unqualified tables", "drop the DBName assignment in readerConfig", "red: an unqualified table fails with no database selected", "TestMySQLDeclaredDatabaseResolvesUnqualifiedTablesAndWithoutItOnlyQualifiedNamesRead"},
}

func TestDatabaseIntegrationMutationRecordMatchesTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "integration_database_test.go", databaseIntegrationMutations)
}

func TestMySQLDeclaredDatabaseResolvesUnqualifiedTablesAndWithoutItOnlyQualifiedNamesRead(t *testing.T) {
	ctx := integration(t)
	target := startMySQL(ctx, t,
		"CREATE TABLE app.items (id INT PRIMARY KEY, name VARCHAR(32))",
		"INSERT INTO app.items VALUES (1, 'in-app')",
		"CREATE DATABASE other",
		"CREATE TABLE other.items (id INT PRIMARY KEY, name VARCHAR(32))",
		"INSERT INTO other.items VALUES (1, 'in-other')",
	)
	decl := contract.Database{Name: "main", Driver: contract.DriverMySQL, ReadSchemas: []string{"app"}, RootSecret: "ROOT_PASSWORD", Database: "app"}
	selected := openPort(t, decl, target)
	res, err := selected.Query(ctx, Request{DB: "main", Statement: "SELECT name, DATABASE() FROM items"})
	if err != nil || len(res.Rows) != 1 || res.Rows[0][0] != "in-app" || res.Rows[0][1] != "app" {
		t.Fatalf("unqualified table with database app: %+v %v", res, err)
	}
	decl.Database = ""
	plain := openPort(t, decl, target)
	if _, err := plain.Query(ctx, Request{DB: "main", Statement: "SELECT name FROM items"}); err == nil {
		t.Fatal("an unqualified table resolved without a declared database")
	}
	res, err = plain.Query(ctx, Request{DB: "main", Statement: "SELECT name, DATABASE() FROM app.items"})
	if err != nil || len(res.Rows) != 1 || res.Rows[0][0] != "in-app" || res.Rows[0][1] != nil {
		t.Fatalf("qualified table without a database: %+v %v", res, err)
	}
	decl.Database = "other"
	outside := openPort(t, decl, target)
	if _, err := outside.Query(ctx, Request{DB: "main", Statement: "SELECT name FROM items"}); codeOf(err) != engine.PortDenied {
		t.Fatalf("a declared database outside the read schemas: %v", err)
	}
}
