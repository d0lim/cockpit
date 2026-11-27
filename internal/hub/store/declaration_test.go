package store

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
)

func TestARunRecordWrittenWithoutADeclarationReadsAsTheDefaultAndOneWithAFileKeepsIt(t *testing.T) {
	s := Open(homeFixture(t))
	old, err := json.Marshal(fixture(runA))
	if err != nil || strings.Contains(string(old), "declaration") {
		t.Fatalf("a record of the default declaration names one: %s, %v", old, err)
	}
	dir, err := s.RunDir(runA)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "meta.json"), old, 0o600); err != nil {
		t.Fatal(err)
	}
	m, err := s.ReadMeta(runA)
	if err != nil || m == nil || m.Declaration != "" || m.Declaration.Path() != contract.DefaultDeclaration {
		t.Fatalf("the record without a declaration read as %+v, %v", m, err)
	}
	mustWriteMeta(t, s, fixture(runB, func(m *Meta) { m.Declaration = ".cockpit/alt.yaml" }))
	alt, err := s.ReadMeta(runB)
	if err != nil || alt.Declaration.Path() != ".cockpit/alt.yaml" {
		t.Fatalf("the record with a declaration read as %+v, %v", alt, err)
	}
}

func TestAnImportedRunKeepsTheDeclarationItsBundleNames(t *testing.T) {
	alt := bundleMeta("run-20261019-010011-abc1234-h1")
	alt.Declaration = ".cockpit/alt.yaml"
	if got := ImportedMeta(alt).Declaration; got != ".cockpit/alt.yaml" {
		t.Fatalf("imported declaration %q", got)
	}
	if got := ImportedMeta(bundleMeta("run-20261019-010011-abc1234-h2")).Declaration; got.Path() != contract.DefaultDeclaration {
		t.Fatalf("a bundle without a declaration imported as %q", got)
	}
}
