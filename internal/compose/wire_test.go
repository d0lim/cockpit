package compose

import (
	"fmt"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var wireMutations = []porttest.Mutation{
	{"route names match the name pattern", "drop the wire routes from declaredNames", "red: a route name with capitals passes confine", "TestWireRouteNamesOutsideThePatternOrUsedTwiceAreRejectedByConfinementAndGoodNamesPass"},
	{"a declaration loads its wire section", "rename the JSON name of Declaration.Wire", "red: the declared wire section is refused", "TestADeclarationWithWireRoutesAndPublicFieldsLoadsAndOneWithoutThemHasNone"},
	{"a route targets a service of the model", "drop the m.has case in checkWireRoutes", "red: a route to an unknown service passes", "TestWireRoutesAreRefusedAtTheirFieldUnlessTheyNameAProjectServicePortOnce"},
	{"a route does not target the builder", "drop the builder case in checkWireRoutes", "red: a route to the builder passes", "TestWireRoutesAreRefusedAtTheirFieldUnlessTheyNameAProjectServicePortOnce"},
	{"a route does not target a harness service", "drop the HarnessServices case in checkWireRoutes", "red: the refusal moves away from the route field", "TestWireRoutesAreRefusedAtTheirFieldUnlessTheyNameAProjectServicePortOnce"},
	{"a route port is a real port", "drop the port case in checkWireRoutes", "red: ports 0 and 65536 pass", "TestWireRoutesAreRefusedAtTheirFieldUnlessTheyNameAProjectServicePortOnce"},
	{"a service and port pair is routed once", "drop the seen map in checkWireRoutes", "red: the same pair passes twice", "TestWireRoutesAreRefusedAtTheirFieldUnlessTheyNameAProjectServicePortOnce"},
	{"the number of routes is bounded", "drop the limit in checkWireRoutes", "red: 65 routes pass", "TestSixtyFourWireRoutesPassAndOneMoreIsRefused"},
	{"public fields are checked when the declaration is read", "drop checkPublicFields from ReadDeclaration", "red: a public field on password passes", "TestWirePublicFieldsAreRefusedAsSecretsUnlessTheKeyAndValueAreNarrow"},
}

func TestWireMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "wire_test.go", wireMutations)
}

func loadToyWithWire(t *testing.T, wire string, mutate ...func(dir string)) (Loaded, error) {
	t.Helper()
	return loadToy(t, func(d string) {
		path := filepath.Join(d, DeclarationPath)
		writeFile(t, path, readFile(t, path)+"wire:\n"+wire)
		for _, m := range mutate {
			m(d)
		}
	})
}

func TestADeclarationWithWireRoutesAndPublicFieldsLoadsAndOneWithoutThemHasNone(t *testing.T) {
	loaded, err := loadToyWithWire(t, "  routes:\n    - { name: app-api, service: app, port: 8080 }\n    - { name: app-admin, service: app, port: 8081 }\n  publicFields:\n    - { key: token, value: \"tok-[0-9a-f]{8,64}\" }\n")
	if err != nil {
		t.Fatal(err)
	}
	wire := loaded.Declaration.Wire
	if len(wire.Routes) != 2 || wire.Routes[1] != (contract.WireRoute{Name: "app-admin", Service: "app", Port: 8081}) {
		t.Fatalf("routes %+v", wire.Routes)
	}
	if len(wire.PublicFields) != 1 || wire.PublicFields[0].Key != "token" {
		t.Fatalf("public fields %+v", wire.PublicFields)
	}
	if plain := mustLoadToy(t).Declaration.Wire; len(plain.Routes) != 0 || len(plain.PublicFields) != 0 {
		t.Fatalf("the toy declares no wire section, got %+v", plain)
	}
}

func TestWireRouteNamesOutsideThePatternOrUsedTwiceAreRejectedByConfinementAndGoodNamesPass(t *testing.T) {
	loaded := mustLoadToy(t)
	route := func(name contract.Name, port int) contract.WireRoute {
		return contract.WireRoute{Name: name, Service: "app", Port: port}
	}
	bad := []contract.Name{"App", "a_b", "-a", "a/b", "", contract.Name("a" + strings.Repeat("b", 63))}
	for _, name := range bad {
		d := loaded.Declaration
		d.Wire.Routes = []contract.WireRoute{route(name, 8080)}
		r := rejection(t, confine(d, loaded.Tree))
		if r.Kind != RejectConfinement || r.Field != "wire.routes[0].name" {
			t.Errorf("name %q: %v", name, r)
		}
	}
	d := loaded.Declaration
	d.Wire.Routes = []contract.WireRoute{route("api", 8080), route("api", 8081)}
	if r := rejection(t, confine(d, loaded.Tree)); r.Kind != RejectConfinement || r.Field != "wire.routes[1].name" || !strings.Contains(r.Message, "wire.routes[0].name") {
		t.Errorf("repeated name: %v", r)
	}
	d.Wire.Routes = []contract.WireRoute{route("app", 8080), route("a-1", 8081), route(contract.Name("a"+strings.Repeat("b", 62)), 8082)}
	if err := confine(d, loaded.Tree); err != nil {
		t.Fatalf("good names were refused: %v", err)
	}
	_, err := loadToyWithWire(t, "  routes:\n    - { name: App, service: app, port: 8080 }\n")
	if r := rejection(t, err); r.Kind != RejectSchema {
		t.Fatalf("a malformed name in the file: %v", r)
	}
}

func TestWireRoutesAreRefusedAtTheirFieldUnlessTheyNameAProjectServicePortOnce(t *testing.T) {
	addService := func(name string) func(string) {
		return func(d string) {
			replaceIn(t, filepath.Join(d, "compose.yaml"), "\n  cache:\n    image:", "\n  "+name+":\n    image: docker.io/library/alpine:3.22\n  cache:\n    image:")
		}
	}
	cases := []struct {
		name   string
		routes string
		mutate []func(string)
		field  string
	}{
		{"unknown service", "{ name: a, service: queue, port: 80 }", nil, "wire.routes[0].service"},
		{"the builder", "{ name: a, service: builder, port: 80 }", nil, "wire.routes[0].service"},
		{"a harness service", "{ name: a, service: cockpit-otlp, port: 80 }", []func(string){addService("cockpit-otlp")}, "wire.routes[0].service"},
		{"port zero", "{ name: a, service: app, port: 0 }", nil, "wire.routes[0].port"},
		{"port above the range", "{ name: a, service: app, port: 65536 }", nil, "wire.routes[0].port"},
		{"a pair used twice", "{ name: a, service: app, port: 80 }\n    - { name: b, service: app, port: 80 }", nil, "wire.routes[1]"},
	}
	for _, c := range cases {
		_, err := loadToyWithWire(t, "  routes:\n    - "+c.routes+"\n", c.mutate...)
		if r := rejection(t, err); r.Kind != RejectCompose || r.Field != c.field {
			t.Errorf("%s: %v, want compose at %s", c.name, r, c.field)
		}
	}
	allowed := []string{
		"{ name: a, service: app, port: 1 }",
		"{ name: a, service: app, port: 65535 }",
		"{ name: a, service: app, port: 80 }\n    - { name: b, service: app, port: 81 }\n    - { name: c, service: db, port: 80 }",
	}
	for _, routes := range allowed {
		if _, err := loadToyWithWire(t, "  routes:\n    - "+routes+"\n"); err != nil {
			t.Errorf("%s was refused: %v", routes, err)
		}
	}
	if len(cases) != 6 || len(allowed) != 3 {
		t.Fatalf("evaluated %d and %d cases", len(cases), len(allowed))
	}
}

func TestSixtyFourWireRoutesPassAndOneMoreIsRefused(t *testing.T) {
	routes := func(n int) string {
		var b strings.Builder
		b.WriteString("  routes:\n")
		for i := range n {
			fmt.Fprintf(&b, "    - { name: r%d, service: app, port: %d }\n", i, 8000+i)
		}
		return b.String()
	}
	if _, err := loadToyWithWire(t, routes(64)); err != nil {
		t.Fatalf("64 routes were refused: %v", err)
	}
	_, err := loadToyWithWire(t, routes(65))
	if r := rejection(t, err); r.Kind != RejectCompose || r.Field != "wire.routes" {
		t.Fatalf("65 routes: %v", r)
	}
}

func TestWirePublicFieldsAreRefusedAsSecretsUnlessTheKeyAndValueAreNarrow(t *testing.T) {
	denied := []struct {
		name  string
		field string
		says  string
	}{
		{"{ key: password, value: \"tok-[0-9a-f]{8}\" }", "wire.publicFields[0]", "family"},
		{"{ key: client_secret, value: \"tok-[0-9a-f]{8}\" }", "wire.publicFields[0]", "family"},
		{"{ key: email, value: \"tok-[0-9a-f]{8}\" }", "wire.publicFields[0]", "family"},
		{"{ key: token, value: \"[0-9a-f]+\" }", "wire.publicFields[0]", "matches"},
		{"{ key: token, value: \".*\" }", "wire.publicFields[0]", "matches"},
		{"{ key: token, value: \"(\" }", "wire.publicFields[0]", "RE2"},
		{"{ key: token, value: \"tok-[0-9]{4}\" }\n    - { key: session, value: \"[a-z]+\" }", "wire.publicFields[1]", "matches"},
	}
	for _, c := range denied {
		_, err := loadToyWithWire(t, "  publicFields:\n    - "+c.name+"\n")
		r := rejection(t, err)
		if r.Kind != RejectSecrets || r.Field != c.field || !strings.Contains(r.Message, c.says) {
			t.Errorf("%s: %v", c.name, r)
		}
	}
	allowed := []string{"{ key: token, value: \"tok-[0-9a-f]{8,64}\" }", "{ key: Session, value: \"sess-[a-z]{6}\" }", "{ key: credential, value: \"cred/[0-9]+\" }"}
	for _, f := range allowed {
		if _, err := loadToyWithWire(t, "  publicFields:\n    - "+f+"\n"); err != nil {
			t.Errorf("%s was refused: %v", f, err)
		}
	}
	if len(denied) != 7 || len(allowed) != 3 {
		t.Fatalf("evaluated %d and %d cases", len(denied), len(allowed))
	}
}
