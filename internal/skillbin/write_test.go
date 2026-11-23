package skillbin

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/archive"
	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/hub/cli"
)

var (
	frontName   = regexp.MustCompile(`(?m)^name: (\S+)$`)
	frontAbout  = regexp.MustCompile(`(?m)^description: (.+)$`)
	formatWords = regexp.MustCompile(`\*\*(archive|summary) format (\d+)\*\*`)
	reportPath  = regexp.MustCompile(`\breport/([A-Za-z0-9_.<>/-]*)`)
	skillLink   = regexp.MustCompile(`\]\(([a-z0-9.-]+\.md)\)`)
	runsCommand = regexp.MustCompile("`cockpit runs ([a-z]+)")
	hubCommand  = regexp.MustCompile("`cockpit [a-z]")
	holes       = strings.NewReplacer("<take>", "suite", "<name>", "alpha-1", "<file>", "flows/alpha.ts")
	samples     = strings.NewReplacer("/**", "/flows/alpha.ts", "/*/", "/suite/", "*.json", "alpha-1.json")
)

func skillText(t *testing.T) (map[string]string, string) {
	t.Helper()
	names, err := Names()
	if err != nil {
		t.Fatal(err)
	}
	files := map[string]string{}
	var all strings.Builder
	for _, name := range names {
		data, err := Read(name)
		if err != nil {
			t.Fatal(err)
		}
		files[name] = string(data)
		all.Write(data)
	}
	return files, all.String()
}

func TestWritePutsTheSkillUnderItsNameAndReplacesWhatItWroteBefore(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "skills")
	written, err := Write(dir)
	if err != nil {
		t.Fatal(err)
	}
	want := []string{filepath.Join(Name, Entry), filepath.Join(Name, "reading.md")}
	if !slices.Equal(written, want) {
		t.Fatalf("written %q", written)
	}
	entry := filepath.Join(dir, Name, Entry)
	if err := os.WriteFile(entry, []byte("stale"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := Write(dir); err != nil {
		t.Fatal(err)
	}
	embedded, err := Read(Entry)
	onDisk, readErr := os.ReadFile(entry)
	if err != nil || readErr != nil || !bytes.Equal(embedded, onDisk) || len(onDisk) < 500 {
		t.Fatalf("the entry was not replaced: %v %v", err, readErr)
	}
}

func TestTheSkillEntryNamesItselfAndEveryFileItPointsToIsWritten(t *testing.T) {
	files, _ := skillText(t)
	entry := files[Entry]
	if !strings.HasPrefix(entry, "---\n") || frontName.FindStringSubmatch(entry) == nil || frontName.FindStringSubmatch(entry)[1] != Name {
		t.Fatalf("the entry does not name the skill %s", Name)
	}
	if about := frontAbout.FindStringSubmatch(entry); about == nil || len(about[1]) < 80 || len(about[1]) > 1024 {
		t.Fatalf("description %q", about)
	}
	linked := 0
	for _, m := range skillLink.FindAllStringSubmatch(entry, -1) {
		if files[m[1]] == "" {
			t.Errorf("the entry points to %s, which is not written", m[1])
		}
		linked++
	}
	if linked != len(files)-1 || linked == 0 {
		t.Fatalf("%d links for %d files", linked, len(files))
	}
}

func TestTheSkillNamesTheFormatsAndKindsTheArchiveWritesBothWays(t *testing.T) {
	files, all := skillText(t)
	formats := map[string]string{}
	for _, m := range formatWords.FindAllStringSubmatch(files[Entry], -1) {
		formats[m[1]] = m[2]
	}
	if formats["archive"] != strconv.Itoa(int(contract.ArchiveFormat1)) || formats["summary"] != strconv.Itoa(int(contract.SummaryFormat3)) || len(formats) != 2 {
		t.Fatalf("formats %v", formats)
	}
	named := map[contract.ArchiveKind]bool{}
	paths := 0
	for _, m := range reportPath.FindAllStringSubmatch(all, -1) {
		rel := holes.Replace(m[1])
		paths++
		if rel == "" || strings.HasSuffix(rel, "/") {
			under := false
			for _, k := range archive.Kinds {
				under = under || strings.HasPrefix(samples.Replace(k.Path), rel)
			}
			if !under {
				t.Errorf("report/%s is no directory of the archive", m[1])
			}
			continue
		}
		kind, ok := archive.KindOf(rel)
		if !ok {
			t.Errorf("report/%s is no file of the archive", m[1])
		}
		named[kind.Name] = true
	}
	for _, k := range archive.Kinds {
		if sample, ok := archive.KindOf(samples.Replace(k.Path)); !ok || sample.Name != k.Name {
			t.Fatalf("the sample path of kind %s is not of that kind", k.Name)
		}
		if !named[k.Name] {
			t.Errorf("the skill names no file of kind %s", k.Name)
		}
	}
	if paths < len(archive.Kinds) || len(named) != len(archive.Kinds) {
		t.Fatalf("%d paths name %d kinds", paths, len(named))
	}
	for _, at := range []string{archive.FileIndex, archive.DirSchema + "/"} {
		if !strings.Contains(all, "`"+at+"`") {
			t.Errorf("the skill does not name %s", at)
		}
	}
}

func TestTheSkillReadsAnArchiveWithoutTheBinaryAndNamesOnlyCommandsTheBinaryHas(t *testing.T) {
	files, all := skillText(t)
	var usage bytes.Buffer
	if code := cli.Main(context.Background(), []string{"runs", "--help"}, cli.IO{Getenv: func(string) string { return "" }, Stdout: &usage, Stderr: &usage}); code != cli.ExitOK {
		t.Fatalf("usage: %d", code)
	}
	commands := 0
	for _, m := range runsCommand.FindAllStringSubmatch(all, -1) {
		commands++
		if !strings.Contains(usage.String(), " "+m[1]+" ") {
			t.Errorf("cockpit runs %s is not in the usage", m[1])
		}
	}
	if commands != 1 || len(hubCommand.FindAllString(all, -1)) != commands {
		t.Fatalf("the skill names %d runs commands among %d cockpit commands", commands, len(hubCommand.FindAllString(all, -1)))
	}
	blocks := 0
	for name, text := range files {
		parts := strings.Split(text, "```")
		for i := 1; i < len(parts); i += 2 {
			blocks++
			for _, line := range strings.Split(parts[i], "\n") {
				if word := strings.Fields(line); len(word) > 0 && word[0] == "cockpit" {
					t.Errorf("%s runs %q", name, line)
				}
			}
		}
	}
	if blocks < 10 {
		t.Fatalf("only %d command blocks were read", blocks)
	}
}
