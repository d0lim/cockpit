package compose

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
	"github.com/d0lim/cockpit/internal/version"
)

var declarationMutations = []porttest.Mutation{
	{"an unknown host API is rejected", "drop the ServedHostAPIs check in ReadDeclaration", "red: hostApi 2 is accepted", "TestToyDeclarationPassesAndHostApi2MissingSecretsMissingFileAndBadYAMLAreRejectedForDifferentReasons"},
	{"a declaration without secret sources is rejected", "drop the empty secrets.sources check", "red: secrets.sources [] is accepted", "TestToyDeclarationPassesAndHostApi2MissingSecretsMissingFileAndBadYAMLAreRejectedForDifferentReasons"},
	{"the schema gate runs before decoding", "skip schema.Validate in ParseDeclaration", "red: a declaration without secrets reaches the secrets check instead of the schema", "TestToyDeclarationPassesAndHostApi2MissingSecretsMissingFileAndBadYAMLAreRejectedForDifferentReasons"},
	{"offsets stay inside the block and off reserved offsets", "drop the range and reserved checks in checkOffsets", "red: offsets 100, -1 and 99 pass", "TestOffsetsOutsideTheBlockOnTheReservedCanaryOrTakenTwiceAreRejectedAndInBlockOffsetsPass"},
	{"a database names the offset that publishes it", "drop checkDatabasePorts from ReadDeclaration", "red: a database port naming no offset is accepted", "TestDatabasePortNamingNoOffsetIsRejectedAtItsFieldAndANamedOffsetPasses"},
	{"consumer services cannot use the runner repository", "make usesRunnerRepository return false", "red: a service on cockpit-runner:abc is accepted", "TestConsumerServiceOnTheRunnerRepositoryIsRejectedAndOtherImagesPass"},
	{"harness names and missing services are rejected", "drop the reserved-name and service checks in Model.check", "red: a consumer edge service or an unknown builder passes", "TestReservedHarnessNamesAndServicesMissingFromComposeAreRejected"},
}

func TestDeclarationMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "declaration_test.go", declarationMutations)
}

func TestToyDeclarationPassesAndHostApi2MissingSecretsMissingFileAndBadYAMLAreRejectedForDifferentReasons(t *testing.T) {
	decl := func(dir string) string { return filepath.Join(dir, DeclarationPath) }
	cases := []struct {
		name   string
		mutate func(dir string)
		kind   RejectKind
		says   []string
	}{
		{"host API 2", func(d string) { replaceIn(t, decl(d), "hostApi: 1", "hostApi: 2") }, RejectHostAPI, []string{"host API version 2", "serves host API 1", version.String()}},
		{"secrets section removed", func(d string) {
			replaceIn(t, decl(d), "secrets:\n  sources:\n    - { path: .env, format: dotenv }\n  publicNames: [POSTGRES_USER, POSTGRES_DB]\n  patterns: [\"toy_[A-Za-z0-9]{24}\"]\n", "")
		}, RejectSchema, []string{"secrets"}},
		{"secret sources empty", func(d string) {
			replaceIn(t, decl(d), "  sources:\n    - { path: .env, format: dotenv }\n", "  sources: []\n")
		}, RejectSecrets, []string{"secrets.sources"}},
		{"declaration file absent", func(d string) { os.Remove(decl(d)) }, RejectMissing, []string{DeclarationPath, "hostApi 1"}},
		{"YAML syntax error", func(d string) { replaceIn(t, decl(d), "project:\n", "project: [\n") }, RejectUnreadable, []string{"not valid YAML"}},
	}
	if _, err := loadToy(t, nil); err != nil {
		t.Fatalf("the original toy declaration was rejected: %v", err)
	}
	kinds := map[RejectKind]bool{}
	for _, c := range cases {
		_, err := loadToy(t, c.mutate)
		r := rejection(t, err)
		if r.Kind != c.kind {
			t.Errorf("%s: kind %s, want %s (%v)", c.name, r.Kind, c.kind, r)
		}
		for _, s := range c.says {
			if !strings.Contains(r.Error(), s) {
				t.Errorf("%s: message %q does not say %q", c.name, r.Error(), s)
			}
		}
		kinds[r.Kind] = true
	}
	if len(kinds) != len(cases) {
		t.Fatalf("%d cases produced %d distinct reasons", len(cases), len(kinds))
	}
}

func TestOffsetsOutsideTheBlockOnTheReservedCanaryOrTakenTwiceAreRejectedAndInBlockOffsetsPass(t *testing.T) {
	port := func(name string, offset int) contract.PortOffset {
		return contract.PortOffset{Name: contract.Name(name), Offset: offset, Service: "app", Port: 8080, Display: name, Open: contract.OpenLink}
	}
	if err := checkOffsets([]contract.PortOffset{port("a", 0), port("b", 1), port("c", 98)}); err != nil {
		t.Fatalf("in-block offsets were rejected: %v", err)
	}
	cases := []struct {
		name  string
		ports []contract.PortOffset
		field string
	}{
		{"offset at the block width", []contract.PortOffset{port("a", PortWidth)}, "ports[0].offset"},
		{"negative offset", []contract.PortOffset{port("a", -1)}, "ports[0].offset"},
		{"reserved canary offset", []contract.PortOffset{port("a", CanaryOffset)}, "ports[0].offset"},
		{"offset taken twice", []contract.PortOffset{port("a", 3), port("b", 3)}, "ports[1].offset"},
		{"port zero", []contract.PortOffset{{Name: "a", Offset: 4, Service: "app", Port: 0}}, "ports[0].port"},
	}
	for _, c := range cases {
		r := rejection(t, checkOffsets(c.ports))
		if r.Kind != RejectPorts || r.Field != c.field {
			t.Errorf("%s: %v, want ports rejection at %s", c.name, r, c.field)
		}
	}
	_, err := loadToy(t, func(d string) {
		replaceIn(t, filepath.Join(d, DeclarationPath), "offset: 2,", "offset: 120,")
	})
	if r := rejection(t, err); r.Kind != RejectPorts || r.Field != "ports[2].offset" {
		t.Fatalf("declaration with offset 120: %v", r)
	}
}

func TestDatabasePortNamingNoOffsetIsRejectedAtItsFieldAndANamedOffsetPasses(t *testing.T) {
	denied := []string{"queue", "main", "db2"}
	for _, name := range denied {
		_, err := loadToy(t, func(d string) {
			replaceIn(t, filepath.Join(d, DeclarationPath), "port: db, readSchemas", "port: "+name+", readSchemas")
		})
		r := rejection(t, err)
		if r.Kind != RejectPorts || r.Field != "databases[0].port" || !strings.Contains(r.Message, `"`+name+`" names no entry of ports`) {
			t.Errorf("port %s: %v", name, r)
		}
	}
	allowed := []string{"db", "cache"}
	for _, name := range allowed {
		loaded, err := loadToy(t, func(d string) {
			replaceIn(t, filepath.Join(d, DeclarationPath), "port: db, readSchemas", "port: "+name+", readSchemas")
		})
		if err != nil || string(loaded.Declaration.Databases[0].Port) != name {
			t.Errorf("port %s: %+v %v", name, loaded.Declaration.Databases, err)
		}
	}
	if err := checkDatabasePorts(contract.Declaration{}); err != nil {
		t.Fatalf("a declaration without databases: %v", err)
	}
}

func TestConsumerServiceOnTheRunnerRepositoryIsRejectedAndOtherImagesPass(t *testing.T) {
	uses := []string{"cockpit-runner", "cockpit-runner:abc123def456", "localhost:5000/cockpit-runner:x", "docker.io/library/cockpit-runner@sha256:00", "cockpit-runner:${TAG}"}
	others := []string{"toy/app:${IMG_APP:?}", "docker.io/library/postgres:17", "cockpit-runner-extra:1", "my-cockpit-runner", ""}
	for _, image := range uses {
		if !usesRunnerRepository(image) {
			t.Errorf("%q was not seen as the runner repository", image)
		}
	}
	for _, image := range others {
		if usesRunnerRepository(image) {
			t.Errorf("%q was seen as the runner repository", image)
		}
	}
	_, err := loadToy(t, func(d string) {
		replaceIn(t, filepath.Join(d, "compose.sealed-app.yaml"), "image: toy/app:${IMG_APP:?}", "image: cockpit-runner:abc123def456")
	})
	if r := rejection(t, err); r.Kind != RejectCompose || r.Field != "services.app.image" {
		t.Fatalf("app on the runner repository: %v", r)
	}
}

func TestReservedHarnessNamesAndServicesMissingFromComposeAreRejected(t *testing.T) {
	cases := []struct {
		name  string
		file  string
		old   string
		new   string
		kind  RejectKind
		field string
	}{
		{"consumer edge service", "compose.yaml", "\n  cache:\n    image:", "\n  edge:\n    image: docker.io/library/alpine:3.22\n  cache:\n    image:", RejectCompose, "services.edge"},
		{"consumer egress network", "compose.yaml", "volumes:\n  db-data:\n", "volumes:\n  db-data:\nnetworks:\n  egress: {}\n", RejectCompose, "networks.egress"},
		{"unknown builder", DeclarationPath, "builder: builder", "builder: compiler", RejectCompose, "project.builder"},
		{"unknown expected service", DeclarationPath, "expectedServices: [app, db, cache, seed]", "expectedServices: [app, db, queue, seed]", RejectCompose, "project.expectedServices[2]"},
		{"port on an unknown service", DeclarationPath, "service: cache,", "service: queue,", RejectPorts, "ports[2].service"},
	}
	for _, c := range cases {
		_, err := loadToy(t, func(d string) { replaceIn(t, filepath.Join(d, c.file), c.old, c.new) })
		if r := rejection(t, err); r.Kind != c.kind || r.Field != c.field {
			t.Errorf("%s: %v, want %s at %s", c.name, r, c.kind, c.field)
		}
	}
	loaded := mustLoadToy(t)
	if got := strings.Join(loaded.Model.ServiceNames(), ","); got != "app,builder,cache,db,seed" {
		t.Fatalf("toy services %s", got)
	}
}
