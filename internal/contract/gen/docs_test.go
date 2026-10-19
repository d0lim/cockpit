package main

import (
	"strings"
	"testing"
)

const sample = "declare module \"m\" {\n\texport interface Port {\n\t\treadonly ready: boolean;\n\t\tsend(x: string): void;\n\t}\n\texport function helper(): void;\n}\n"

func TestDocsAreInsertedAboveTopLevelDeclarationsAndInterfaceMembers(t *testing.T) {
	used := map[string]bool{}
	got := withDocs(sample, map[string]string{"Port": "P.", "Port.send": "S.", "Port.ready": "R.", "helper": "H."}, used)
	for _, want := range []string{"\t/** P. */\n\texport interface Port", "\t\t/** R. */\n\t\treadonly ready", "\t\t/** S. */\n\t\tsend(", "\t/** H. */\n\texport function helper"} {
		if !strings.Contains(got, want) {
			t.Fatalf("missing %q in\n%s", want, got)
		}
	}
	if len(used) != 4 {
		t.Fatalf("used %v", used)
	}
}

func TestMemberKeysDoNotMatchOutsideTheirInterface(t *testing.T) {
	used := map[string]bool{}
	got := withDocs(sample, map[string]string{"Other.send": "X."}, used)
	if got != sample || len(used) != 0 {
		t.Fatalf("unexpected insertion:\n%s", got)
	}
}

func TestUnusedDocKeysFailGenerationAndNameEachKey(t *testing.T) {
	used := map[string]bool{"Port": true}
	err := unusedDocs(map[string]string{"Port": "P.", "Gone": "G.", "Port.lost": "L."}, used)
	if err == nil || !strings.Contains(err.Error(), "Gone, Port.lost") {
		t.Fatalf("err %v", err)
	}
	if unusedDocs(map[string]string{"Port": "P."}, used) != nil {
		t.Fatal("a fully used table was rejected")
	}
}
