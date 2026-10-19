package sqlport

import (
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var mysqlMutations = []porttest.Mutation{
	{"the declared database becomes the MySQL reader's default database", "drop the DBName assignment in readerConfig", "red: the reader config has no database", "TestMySQLReaderConfigSelectsTheDeclaredDatabaseAndNoneWhenItIsOmitted"},
}

func TestMySQLMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "mysql_test.go", mysqlMutations)
}

func TestMySQLReaderConfigSelectsTheDeclaredDatabaseAndNoneWhenItIsOmitted(t *testing.T) {
	cases := map[string]string{"app": "app", "": ""}
	checked := 0
	for declared, want := range cases {
		d := contract.Database{Name: "main", Driver: contract.DriverMySQL, Port: "db", ReadSchemas: []string{"app"}, RootSecret: "R", Database: declared}
		p, err := New(Options{Databases: []contract.Database{d}, Targets: map[string]Target{"main": {Host: "127.0.0.1", Port: 3306}}})
		if err != nil {
			t.Fatal(err)
		}
		b, ok := p.dbs["main"].backend.(mysqlBackend)
		if !ok {
			t.Fatalf("backend %T", p.dbs["main"].backend)
		}
		if got := b.readerConfig("pw").DBName; got != want {
			t.Errorf("declared %q: reader database %q", declared, got)
		}
		if got := b.config("root", "pw").DBName; got != "" {
			t.Errorf("declared %q: the provisioning connection selects %q", declared, got)
		}
		checked++
	}
	if checked != 2 {
		t.Fatalf("checked %d cases", checked)
	}
}
