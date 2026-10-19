package envops

import (
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

func TestDatabaseTargetsResolveEachDatabaseThroughItsDeclaredPortAndRefuseAnUnpublishedOne(t *testing.T) {
	decl := contract.Declaration{Databases: []contract.Database{
		{Name: "main", Driver: contract.DriverPostgres, Port: "db", RootUser: "toy", Database: "toy"},
		{Name: "orders", Driver: contract.DriverMySQL, Port: "orders-db"},
	}}
	targets, err := DatabaseTargets(decl, map[string]int{"db": 22011, "orders-db": 22012, "app": 22001})
	if err != nil {
		t.Fatal(err)
	}
	if len(targets) != 2 || targets["main"].Port != 22011 || targets["main"].Host != "127.0.0.1" || targets["main"].Database != "toy" || targets["main"].RootUser != "toy" || targets["orders"].Port != 22012 {
		t.Fatalf("targets %+v", targets)
	}
	decl.Databases[1].Port = "missing"
	if got, err := DatabaseTargets(decl, map[string]int{"db": 22011, "app": 22001}); err == nil || got != nil || !strings.Contains(err.Error(), "databases[1].port") {
		t.Fatalf("an unpublished port must refuse the run's sql port, got %+v %v", got, err)
	}
}
