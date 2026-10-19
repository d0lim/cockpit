package compose

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"path/filepath"
	"slices"
	"strings"
	"testing"
)

func toyKeys(t *testing.T, dir, sha string) map[string]string {
	t.Helper()
	loaded, err := ReadDeclaration(context.Background(), Git{Dir: dir}, sha)
	if err != nil {
		t.Fatal(err)
	}
	keys, err := ImageKeys(loaded.Declaration, loaded.Tree)
	if err != nil {
		t.Fatal(err)
	}
	return keys
}

func TestImageKeyChangesWhenOneInputTreeChangesAndStaysWhenAFileOutsideTheInputsChanges(t *testing.T) {
	dir, base := toyRepo(t, nil)
	first := toyKeys(t, dir, base)
	if len(first) != 1 || len(first["IMG_APP"]) != 12 {
		t.Fatalf("keys %v", first)
	}
	outside := commit(t, dir, func() { writeFile(t, filepath.Join(dir, "README.md"), "toy\n") })
	scripts := commit(t, dir, func() { writeFile(t, filepath.Join(dir, "scripts", "extra.ts"), "export {};\n") })
	if got := toyKeys(t, dir, outside); got["IMG_APP"] != first["IMG_APP"] {
		t.Fatalf("a README change moved the key: %v -> %v", first, got)
	}
	if got := toyKeys(t, dir, scripts); got["IMG_APP"] != first["IMG_APP"] {
		t.Fatalf("a scripts change moved the key: %v -> %v", first, got)
	}
	input := commit(t, dir, func() { writeFile(t, filepath.Join(dir, "mock", "extra.go"), "package main\n") })
	moved := toyKeys(t, dir, input)
	if moved["IMG_APP"] == first["IMG_APP"] {
		t.Fatal("a change inside mock/ kept the key")
	}
	composed := commit(t, dir, func() {
		writeFile(t, filepath.Join(dir, "compose.yaml"), readFile(t, filepath.Join(dir, "compose.yaml"))+"\n")
	})
	if got := toyKeys(t, dir, composed); got["IMG_APP"] == moved["IMG_APP"] {
		t.Fatal("a change to a declared compose file kept the key")
	}
}

func TestImageKeyIsTheFirst12HexOfSha256OverSortedTreeIdsWithoutARunnerKey(t *testing.T) {
	loaded := mustLoadToy(t)
	inputs := KeyInputs(loaded.Declaration)
	if got := strings.Join(inputs["IMG_APP"], ","); got != "compose.sealed-app.yaml,compose.yaml,mock" {
		t.Fatalf("inputs %s", got)
	}
	var ids []string
	for _, p := range inputs["IMG_APP"] {
		ids = append(ids, loaded.Tree.Entries[p].ID)
	}
	slices.Sort(ids)
	sum := sha256.Sum256([]byte(strings.Join(ids, "\n")))
	keys, err := ImageKeys(loaded.Declaration, loaded.Tree)
	if err != nil {
		t.Fatal(err)
	}
	if keys["IMG_APP"] != hex.EncodeToString(sum[:])[:12] {
		t.Fatalf("key %s", keys["IMG_APP"])
	}
	for key := range keys {
		if strings.Contains(key, "RUNNER") {
			t.Fatalf("a runner key %s was derived", key)
		}
	}
	env, err := RunEnv(RunEnvInput{RunID: runID, Layout: testLayout(t), PortBase: 22000, Ports: loaded.Declaration.Ports, Keys: keys})
	if err != nil || strings.Contains(env, "RUNNER") || !strings.Contains(env, "IMG_APP="+keys["IMG_APP"]+"\n") {
		t.Fatalf("run.env %q %v", env, err)
	}
	loaded.Declaration.Images[0].Inputs = append(loaded.Declaration.Images[0].Inputs, "missing-dir")
	if _, err := ImageKeys(loaded.Declaration, loaded.Tree); err == nil || !strings.Contains(err.Error(), "missing-dir") {
		t.Fatalf("an absent input: %v", err)
	}
}

func TestRunEnvCarriesRunPathsPortsAndKeysAndRefusesEmptyValues(t *testing.T) {
	l := testLayout(t)
	loaded := mustLoadToy(t)
	env, err := RunEnv(RunEnvInput{RunID: runID, Layout: l, PortBase: 22200, Ports: loaded.Declaration.Ports, Keys: map[string]string{"IMG_APP": "abcdef012345"}})
	if err != nil {
		t.Fatal(err)
	}
	want := "RUN_ID=" + runID + "\nRUN_SRC=" + l.Src + "\nRUN_BUILD=" + l.Build + "\nRUN_WORK=" + l.Work +
		"\nRUN_PORT_APP=22200\nRUN_PORT_DB=22201\nRUN_PORT_CACHE=22202\nIMG_APP=abcdef012345\n"
	if env != want {
		t.Fatalf("run.env\n%s\nwant\n%s", env, want)
	}
	if _, err := RunEnv(RunEnvInput{RunID: runID, Layout: l, Keys: map[string]string{"IMG_APP": ""}}); err == nil || !strings.Contains(err.Error(), "IMG_APP") {
		t.Fatalf("empty key: %v", err)
	}
}
