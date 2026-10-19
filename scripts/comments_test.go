package scripts

import (
	"bufio"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
)

var skippedDirs = map[string]bool{".git": true, "node_modules": true, "bin": true, "dist": true}

var generatedDirs = map[string]bool{"sdk": true}

var hashCommentExts = map[string]bool{".sh": true, ".yaml": true, ".yml": true, ".toml": true}

var slashCommentExts = map[string]bool{".ts": true, ".tsx": true, ".js": true, ".css": true}

type comment struct {
	path string
	line int
	text string
}

func (c comment) String() string {
	return c.path + ":" + strconv.Itoa(c.line) + ": " + c.text
}

func commentsIn(root string) ([]comment, int, error) {
	var found []comment
	scanned := 0
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if path != root && skippedDirs[d.Name()] {
				return filepath.SkipDir
			}
			if rel, _ := filepath.Rel(root, path); generatedDirs[rel] {
				return filepath.SkipDir
			}
			return nil
		}
		ext := filepath.Ext(path)
		var cs []comment
		switch {
		case ext == ".go":
			cs, err = goComments(path)
		case slashCommentExts[ext]:
			cs, err = lineComments(path, []string{"//", "/*", "{/*"}, false)
		case hashCommentExts[ext] || d.Name() == "Makefile":
			cs, err = lineComments(path, []string{"#"}, true)
		default:
			return nil
		}
		if err != nil {
			return err
		}
		scanned++
		found = append(found, cs...)
		return nil
	})
	return found, scanned, err
}

func goComments(path string) ([]comment, error) {
	fset := token.NewFileSet()
	f, err := parser.ParseFile(fset, path, nil, parser.ParseComments)
	if err != nil {
		return nil, err
	}
	var cs []comment
	for _, group := range f.Comments {
		for _, c := range group.List {
			if strings.HasPrefix(c.Text, "//go:") {
				continue
			}
			cs = append(cs, comment{path, fset.Position(c.Pos()).Line, c.Text})
		}
	}
	return cs, nil
}

func lineComments(path string, markers []string, shebang bool) ([]comment, error) {
	f, err := os.Open(path)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	var cs []comment
	sc := bufio.NewScanner(f)
	sc.Buffer(make([]byte, 1<<20), 1<<20)
	n := 0
	for sc.Scan() {
		n++
		t := strings.TrimSpace(sc.Text())
		if n == 1 && shebang && strings.HasPrefix(t, "#!") {
			continue
		}
		for _, m := range markers {
			if strings.HasPrefix(t, m) {
				cs = append(cs, comment{path, n, t})
				break
			}
		}
	}
	return cs, sc.Err()
}

func plant(t *testing.T, rel, content string) string {
	t.Helper()
	root := t.TempDir()
	path := filepath.Join(root, rel)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestTreeHasNoCommentsBeyondGoDirectivesAndShebangs(t *testing.T) {
	cs, scanned, err := commentsIn(repoRoot(t))
	if err != nil {
		t.Fatal(err)
	}
	if scanned == 0 {
		t.Fatal("scanned 0 files")
	}
	if len(cs) > 0 {
		lines := make([]string, 0, len(cs))
		for _, c := range cs {
			lines = append(lines, c.String())
		}
		t.Fatalf("comments found in %d of %d scanned files:\n%s", len(cs), scanned, strings.Join(lines, "\n"))
	}
}

func TestPlantedGoCommentIsReportedWithItsLine(t *testing.T) {
	root := plant(t, "internal/x/x.go", "package x\n\nvar a = 1\n\n// planted\nvar b = 2\n")
	cs, scanned, err := commentsIn(root)
	if err != nil || scanned != 1 || len(cs) != 1 || cs[0].line != 5 || !strings.Contains(cs[0].text, "planted") {
		t.Fatalf("scanned %d, comments %v, err %v", scanned, cs, err)
	}
}

func TestGoDirectivesAreNotComments(t *testing.T) {
	root := plant(t, "x.go", "package x\n\nimport \"embed\"\n\n//go:embed x.go\nvar files embed.FS\n")
	cs, scanned, err := commentsIn(root)
	if err != nil || scanned != 1 || len(cs) != 0 {
		t.Fatalf("scanned %d, comments %v, err %v", scanned, cs, err)
	}
}

func TestShellCommentAfterTheShebangIsReported(t *testing.T) {
	root := plant(t, "scripts/x.sh", "#!/usr/bin/env bash\nset -e\n# planted\necho ok\n")
	cs, scanned, err := commentsIn(root)
	if err != nil || scanned != 1 || len(cs) != 1 || cs[0].line != 3 {
		t.Fatalf("scanned %d, comments %v, err %v", scanned, cs, err)
	}
}

func TestShebangAloneIsNotAComment(t *testing.T) {
	root := plant(t, "scripts/x.sh", "#!/usr/bin/env bash\necho ok\n")
	cs, scanned, err := commentsIn(root)
	if err != nil || scanned != 1 || len(cs) != 0 {
		t.Fatalf("scanned %d, comments %v, err %v", scanned, cs, err)
	}
}

func TestTypeScriptLineCommentIsReported(t *testing.T) {
	root := plant(t, "s/x.ts", "const a = 1;\n// planted\nexport default a;\n")
	cs, scanned, err := commentsIn(root)
	if err != nil || scanned != 1 || len(cs) != 1 || cs[0].line != 2 {
		t.Fatalf("scanned %d, comments %v, err %v", scanned, cs, err)
	}
}

func TestTsxLineAndJsxCommentsAndCssBlockCommentsAreReported(t *testing.T) {
	cases := map[string]string{
		"web/a.tsx":  "export const A = () => (\n\t<div>\n\t\t{/* planted */}\n\t</div>\n);\n",
		"web/b.tsx":  "const b = 1;\n// planted\nexport default b;\n",
		"web/c.css":  "body { margin: 0; }\n/* planted */\n",
		"web/ok.tsx": "export const ok = \"// not at line start\";\n",
	}
	want := map[string]int{"web/a.tsx": 3, "web/b.tsx": 2, "web/c.css": 2, "web/ok.tsx": 0}
	ran := 0
	for rel, content := range cases {
		cs, scanned, err := commentsIn(plant(t, rel, content))
		got := 0
		if len(cs) == 1 {
			got = cs[0].line
		}
		if err != nil || scanned != 1 || len(cs) > 1 || got != want[rel] {
			t.Errorf("%s: scanned %d, comments %v, err %v", rel, scanned, cs, err)
		}
		ran++
	}
	if ran != 4 {
		t.Fatalf("ran %d cases", ran)
	}
}

func TestFilesWithoutCommentSyntaxAreNotScanned(t *testing.T) {
	root := plant(t, "data/x.json", "{\"note\": \"// not a comment\"}\n")
	cs, scanned, err := commentsIn(root)
	if err != nil || scanned != 0 || len(cs) != 0 {
		t.Fatalf("scanned %d, comments %v, err %v", scanned, cs, err)
	}
}

func TestDocCommentsInTheGeneratedSdkDirectoryAreNotScanned(t *testing.T) {
	root := plant(t, "sdk/x.d.ts", "/** generated doc */\nexport {};\n")
	cs, scanned, err := commentsIn(root)
	if err != nil || scanned != 0 || len(cs) != 0 {
		t.Fatalf("scanned %d, comments %v, err %v", scanned, cs, err)
	}
}

func TestDocCommentsInAnSdkDirectoryBelowTheRootAreReported(t *testing.T) {
	root := plant(t, "internal/sdk/x.ts", "/** planted */\nexport {};\n")
	cs, scanned, err := commentsIn(root)
	if err != nil || scanned != 1 || len(cs) != 1 || cs[0].line != 1 {
		t.Fatalf("scanned %d, comments %v, err %v", scanned, cs, err)
	}
}
