package sqlport

import (
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var targetMutations = []porttest.Mutation{
	{"a database reaches the loopback port of the offset it names", "use the first port of the declaration for every database", "red: the cache database resolves to the db offset", "TestEachDatabaseResolvesToTheLoopbackPortOfTheOffsetItNamesAndAnUnknownNameIsRefused"},
	{"a database naming no offset gets no target", "skip databases whose port names no offset", "red: TargetsFor returns targets without an error", "TestEachDatabaseResolvesToTheLoopbackPortOfTheOffsetItNamesAndAnUnknownNameIsRefused"},
	{"targets need the run's port base", "drop the base check in TargetsFor", "red: base 0 resolves to the bare offsets", "TestEachDatabaseResolvesToTheLoopbackPortOfTheOffsetItNamesAndAnUnknownNameIsRefused"},
}

func TestTargetsMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "targets_test.go", targetMutations)
}

func targetDeclaration() contract.Declaration {
	return contract.Declaration{
		Ports: []contract.PortOffset{
			{Name: "app", Offset: 0, Service: "app", Port: 8080},
			{Name: "db", Offset: 1, Service: "db", Port: 5432},
			{Name: "store", Offset: 7, Service: "store", Port: 3306},
		},
		Databases: []contract.Database{
			{Name: "main", Driver: contract.DriverPostgres, Port: "db", ReadSchemas: []string{"public"}, RootSecret: "R"},
			{Name: "orders", Driver: contract.DriverMySQL, Port: "store", ReadSchemas: []string{"app"}, RootSecret: "R", RootUser: "admin", Database: "app"},
		},
	}
}

func TestEachDatabaseResolvesToTheLoopbackPortOfTheOffsetItNamesAndAnUnknownNameIsRefused(t *testing.T) {
	targets, err := TargetsFor(targetDeclaration(), 22400)
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]Target{
		"main":   {Host: "127.0.0.1", Port: 22401},
		"orders": {Host: "127.0.0.1", Port: 22407, RootUser: "admin", Database: "app"},
	}
	if len(targets) != len(want) {
		t.Fatalf("%d targets: %+v", len(targets), targets)
	}
	for name, w := range want {
		if targets[name] != w {
			t.Errorf("%s: %+v, want %+v", name, targets[name], w)
		}
	}
	p, err := New(Options{Databases: targetDeclaration().Databases, Targets: targets})
	if err != nil || len(p.dbs) != 2 {
		t.Fatalf("New with the resolved targets: %v", err)
	}
	refused := 0
	for _, port := range []contract.Name{"cache", "", "main"} {
		decl := targetDeclaration()
		decl.Databases[1].Port = port
		got, err := TargetsFor(decl, 22400)
		if err == nil || got != nil || !strings.Contains(err.Error(), "databases[1].port") {
			t.Errorf("port %q: %+v %v", port, got, err)
		}
		refused++
	}
	for _, base := range []int{0, -100} {
		if got, err := TargetsFor(targetDeclaration(), base); err == nil || got != nil {
			t.Errorf("base %d: %+v", base, got)
		}
		refused++
	}
	if refused != 5 {
		t.Fatalf("evaluated %d refusals", refused)
	}
	none, err := TargetsFor(contract.Declaration{}, 22400)
	if err != nil || len(none) != 0 {
		t.Fatalf("a declaration without databases: %+v %v", none, err)
	}
}
