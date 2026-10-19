package compose

import (
	"fmt"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

const (
	HarnessFile   = "harness.yaml"
	RunEnvFile    = "env/run.env"
	dotenvName    = ".env"
	srcDir        = "src"
	buildDir      = "build"
	workDir       = "work"
	envDir        = "env"
	privateDirMod = 0o700
	privateFile   = 0o600
)

type Layout struct {
	Root  string
	Src   string
	Build string
	Work  string
}

func NewLayout(runDir string) (Layout, error) {
	if !filepath.IsAbs(runDir) {
		return Layout{}, fmt.Errorf("run directory must be an absolute path, got %q", runDir)
	}
	root := filepath.Clean(runDir)
	return Layout{
		Root:  root,
		Src:   filepath.Join(root, srcDir),
		Build: filepath.Join(root, buildDir),
		Work:  filepath.Join(root, workDir),
	}, nil
}

func (l Layout) Harness() string { return filepath.Join(l.Root, HarnessFile) }

func (l Layout) RunEnv() string { return filepath.Join(l.Root, RunEnvFile) }

func AssertSealedFiles(files []string, forbidden []string) error {
	var hits []string
	for _, f := range files {
		base := strings.ToLower(filepath.Base(f))
		for _, stem := range forbidden {
			if stem != "" && strings.Contains(base, strings.ToLower(stem)) {
				hits = append(hits, f)
				break
			}
		}
	}
	if len(hits) > 0 {
		return fmt.Errorf("compose files matching compose.forbidden cannot join a sealed run: %s", strings.Join(hits, ", "))
	}
	return nil
}

func Files(l Layout, decl contract.Declaration) ([]string, error) {
	files := make([]string, 0, len(decl.Compose.Files)+1)
	for _, f := range decl.Compose.Files {
		files = append(files, filepath.Join(l.Src, filepath.FromSlash(string(f))))
	}
	files = append(files, l.Harness())
	if err := AssertSealedFiles(files, decl.Compose.Forbidden); err != nil {
		return nil, err
	}
	return files, nil
}

func projectDir(l Layout, decl contract.Declaration) string {
	return filepath.Join(l.Src, filepath.FromSlash(string(decl.Compose.ProjectDirectory)))
}

func projectDotenv(decl contract.Declaration) contract.RelPath {
	return contract.RelPath(path.Join(string(decl.Compose.ProjectDirectory), dotenvName))
}

func ComposeArgs(project string, l Layout, decl contract.Declaration) ([]string, error) {
	files, err := Files(l, decl)
	if err != nil {
		return nil, err
	}
	args := []string{"compose", "--ansi", "never", "--progress", "plain", "-p", project, "--project-directory", projectDir(l, decl)}
	if dotenv := projectDotenv(decl); slices.Contains(decl.Env.Outputs, dotenv) {
		args = append(args, "--env-file", filepath.Join(l.Src, filepath.FromSlash(string(dotenv))))
	}
	args = append(args, "--env-file", l.RunEnv())
	for _, f := range files {
		args = append(args, "-f", f)
	}
	return args, nil
}

func WriteHarness(root *os.Root, in HarnessInput) error {
	data, err := Harness(in)
	if err != nil {
		return err
	}
	return writePrivate(root, HarnessFile, data)
}
