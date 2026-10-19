package compose

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/mask"
)

const (
	EnvMount       = "/src"
	envRole        = "env"
	envStderrTail  = 5
	fallbackUser   = "65532:65532"
	envProgramName = "cockpit-env"
)

type EnvInput struct {
	RunID  string
	Layout Layout
	Env    contract.EnvSection
	User   string
	Mask   func(string) string
}

func EnvUser(uid, gid int) string {
	if uid <= 0 || gid < 0 {
		return fallbackUser
	}
	return fmt.Sprintf("%d:%d", uid, gid)
}

func EnvArgs(in EnvInput) []string {
	args := []string{
		"run", "--rm", "--network", "none", "--user", in.User,
		"--cap-drop", "ALL", "--security-opt", "no-new-privileges", "--read-only", "--tmpfs", "/tmp",
		"--mount", "type=bind,source=" + in.Layout.Src + ",target=" + EnvMount,
		"--workdir", EnvMount,
		"--env", "COMPOSE_PROJECT_NAME=" + in.RunID,
		"--label", LabelManaged + "=true", "--label", LabelRunID + "=" + in.RunID, "--label", LabelRole + "=" + envRole,
		"--entrypoint", "/bin/sh",
		string(in.Env.Image), "-c", `umask 077 && exec "$@"`, envProgramName,
	}
	return append(args, in.Env.Command...)
}

func srcPath(p contract.RelPath) string {
	return path.Join(srcDir, string(p))
}

func GenerateEnv(ctx context.Context, d Docker, root *os.Root, in EnvInput) error {
	if len(in.Env.Command) == 0 {
		if len(in.Env.Outputs) > 0 {
			return errors.New("env.outputs are declared without env.command")
		}
		return nil
	}
	if in.Env.Image == "" || in.User == "" || in.Mask == nil {
		return errors.New("env generation needs env.image, a user and a masker")
	}
	if strings.ContainsAny(in.Layout.Src, ",=") {
		return fmt.Errorf("run source directory %q cannot be bound: it contains a comma or an equals sign", in.Layout.Src)
	}
	existed := map[contract.RelPath]bool{}
	for _, p := range in.Env.Outputs {
		if _, err := root.Lstat(srcPath(p)); err == nil {
			existed[p] = true
		}
	}
	res, err := d.Run(ctx, Command{Args: EnvArgs(in)})
	if err == nil && res.Code == 0 {
		err = sealOutputs(root, in.Env.Outputs)
		if err == nil {
			return nil
		}
	} else if err == nil {
		err = fmt.Errorf("exit code %d; stderr tail: %s", res.Code, in.Mask(tail(string(res.Stderr), envStderrTail)))
	}
	for _, p := range in.Env.Outputs {
		if !existed[p] {
			root.Remove(srcPath(p))
		}
	}
	return fmt.Errorf("env generation failed and its outputs were removed: %w", err)
}

func sealOutputs(root *os.Root, outputs []contract.RelPath) error {
	for _, p := range outputs {
		info, err := root.Lstat(srcPath(p))
		switch {
		case errors.Is(err, fs.ErrNotExist):
			return fmt.Errorf("env.command did not produce %s", p)
		case err != nil:
			return err
		case !info.Mode().IsRegular():
			return fmt.Errorf("env output %s is not a regular file", p)
		}
		if err := root.Chmod(srcPath(p), privateFile); err != nil {
			return err
		}
	}
	return nil
}

func tail(text string, n int) string {
	var lines []string
	for _, l := range strings.Split(text, "\n") {
		if strings.TrimSpace(l) != "" {
			lines = append(lines, strings.TrimSpace(l))
		}
	}
	if len(lines) == 0 {
		return "(empty)"
	}
	return strings.Join(lines[max(0, len(lines)-n):], " / ")
}

func outputKind(p contract.RelPath) mask.Kind {
	switch path.Ext(string(p)) {
	case ".json":
		return mask.JSON
	case ".pem", ".key":
		return mask.PEM
	}
	return mask.Dotenv
}

func MaskSources(decl contract.Declaration, l Layout) mask.Sources {
	src := mask.Sources{Dir: l.Root, Mode: mask.Hub, PublicNames: slices.Clone(decl.Secrets.PublicNames)}
	for _, p := range decl.Env.Outputs {
		src.Outputs = append(src.Outputs, mask.Source{Kind: outputKind(p), Path: srcPath(p)})
	}
	for _, s := range decl.Secrets.Sources {
		src.Declared = append(src.Declared, mask.Source{Kind: mask.Kind(s.Format), Path: srcPath(s.Path)})
	}
	return src
}
