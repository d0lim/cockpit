package mask

import (
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
)

func tree(files map[string]string) (string, func(), error) {
	dir, err := os.MkdirTemp("", "mask-tree-*")
	if err != nil {
		return "", func() {}, err
	}
	done := func() { os.RemoveAll(dir) }
	for rel, text := range files {
		if err := put(dir, rel, text); err != nil {
			done()
			return "", func() {}, err
		}
	}
	return dir, done, nil
}

func sameSet(label string, got, want []string) []string {
	if !slices.Equal(slices.Sorted(slices.Values(got)), slices.Sorted(slices.Values(want))) {
		return []string{fmt.Sprintf("%s: %q, want %q", label, got, want)}
	}
	return nil
}

func outputsAndPublicScenario(e engine) []string {
	dir, done, err := tree(map[string]string{
		"gen/app.env":     "PUBLIC_URL=http://run-1.localhost:21000\nSIGNING_SECRET=" + planted + "\n",
		"work/extra.json": `{"local":{"baseUrl":"http://run-1.localhost:21001","adminToken":"admin-token-0042"}}`,
	})
	defer done()
	if err != nil {
		return []string{err.Error()}
	}
	v, err := e.refresh(Sources{
		Dir:         dir,
		Mode:        Hub,
		Outputs:     []Source{{Kind: Dotenv, Path: "gen/app.env"}},
		Declared:    []Source{{Kind: JSON, Path: "work/extra.json"}},
		PublicNames: []string{"PUBLIC_URL", "baseUrl"},
	})
	if err != nil {
		return []string{err.Error()}
	}
	f := append(sameSet("value list", v.Known, []string{planted, "admin-token-0042"}), sameSet("missing", v.Missing, nil)...)
	m, err := e.build(Options{Mode: Hub, Known: v.Known})
	if err != nil {
		return append(f, err.Error())
	}
	line := "url=http://run-1.localhost:21000 base=http://run-1.localhost:21001 sig=" + planted + " t=admin-token-0042"
	return append(f, expectEqual("masked line", e.mask(m, line), "url=http://run-1.localhost:21000 base=http://run-1.localhost:21001 sig=****3a9c t=****0042")...)
}

func missingScenario(e engine) []string {
	dir, done, err := tree(map[string]string{
		"gen/app.env":     "MYSQL_ROOT_PASSWORD=${MYSQL_ROOT_PASSWORD}\nEMPTY_TOKEN=\nQUOTED_SECRET=\"${ADMIN_SECRET}\"\nREAL_SECRET=" + planted + "\n",
		"work/extra.json": `{"local":{"seed":"","key":"${SEED_KEY}","ok":"json-value-0099"}}`,
	})
	defer done()
	if err != nil {
		return []string{err.Error()}
	}
	v, err := e.refresh(Sources{
		Dir:      dir,
		Mode:     Hub,
		Outputs:  []Source{{Kind: Dotenv, Path: "gen/app.env"}},
		Declared: []Source{{Kind: JSON, Path: "work/extra.json"}},
	})
	if err != nil {
		return []string{err.Error()}
	}
	f := sameSet("value list", v.Known, []string{planted, "json-value-0099"})
	if want := []string{"gen/app.env#MYSQL_ROOT_PASSWORD", "gen/app.env#EMPTY_TOKEN", "gen/app.env#QUOTED_SECRET", "work/extra.json#local.key", "work/extra.json#local.seed"}; !slices.Equal(v.Missing, want) {
		f = append(f, fmt.Sprintf("missing %q, want %q", v.Missing, want))
	}
	m, err := e.build(Options{Mode: Hub, Known: v.Known})
	if err != nil {
		return append(f, err.Error())
	}
	return append(f, expectEqual("masked line", e.mask(m, "dsn ${MYSQL_ROOT_PASSWORD} "+planted), "dsn ${MYSQL_ROOT_PASSWORD} ****3a9c")...)
}

func refreshFailureScenario(e engine) []string {
	dir, done, err := tree(map[string]string{"gen/app.env": "A_SECRET=" + planted, "conf/x.yml": "a: b"})
	defer done()
	if err != nil {
		return []string{err.Error()}
	}
	cases := []struct {
		source Source
		want   string
	}{
		{Source{Kind: JSON, Path: "work/missing.json"}, "run credential file not found: work/missing.json"},
		{Source{Kind: "yaml", Path: "conf/x.yml"}, `unknown source kind: "yaml" (conf/x.yml)`},
		{Source{Kind: Dotenv, Path: "../escape.env"}, "not a relative path inside the run directory: ../escape.env"},
		{Source{Kind: Dotenv, Path: filepath.Join(dir, "gen/app.env")}, "not a relative path inside the run directory"},
	}
	var f []string
	for _, c := range cases {
		v, err := e.refresh(Sources{Dir: dir, Mode: Hub, Outputs: []Source{{Kind: Dotenv, Path: "gen/app.env"}}, Declared: []Source{c.source}})
		switch {
		case err == nil || !strings.Contains(err.Error(), c.want):
			f = append(f, fmt.Sprintf("%s: error %v, want %q", c.source.Path, err, c.want))
		case v.Known != nil || v.Missing != nil:
			f = append(f, fmt.Sprintf("%s: returned %d values along with the error", c.source.Path, len(v.Known)))
		}
	}
	return f
}
