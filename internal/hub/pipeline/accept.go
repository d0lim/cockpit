package pipeline

import (
	"context"
	"errors"
	"fmt"
	"path/filepath"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/compose"
	"github.com/d0lim/cockpit/internal/hub/scheduler"
)

func rejected(kind scheduler.RejectKind, format string, args ...any) *scheduler.Rejection {
	return &scheduler.Rejection{Kind: kind, Message: fmt.Sprintf(format, args...)}
}

func (h *Hub) Accept(ctx context.Context, in scheduler.SubmitInput) (scheduler.Accepted, error) {
	if !filepath.IsAbs(in.Worktree) {
		return scheduler.Accepted{}, rejected(scheduler.KindWorktree, "the worktree must be an absolute path, got %q", in.Worktree)
	}
	repo, err := h.o.Config.Allow(ctx, compose.CommonDirOf, in.Worktree)
	if err != nil {
		return scheduler.Accepted{}, rejected(scheduler.KindWorktree, "%s", err.Error())
	}
	top, err := compose.Git{Dir: in.Worktree}.TopLevel(ctx)
	if err != nil {
		return scheduler.Accepted{}, rejected(scheduler.KindWorktree, "%s is not inside a worktree", in.Worktree)
	}
	git := compose.Git{Dir: top}
	commit := in.Commit
	if commit == "" {
		commit = "HEAD"
	}
	sha, err := git.ResolveCommit(ctx, commit)
	if err != nil {
		return scheduler.Accepted{}, rejected(scheduler.KindCommit, "%s", err.Error())
	}
	loaded, err := compose.ReadDeclaration(ctx, git, sha)
	var rejection *compose.Rejection
	if errors.As(err, &rejection) && rejection.Kind == compose.RejectHostAPI {
		return scheduler.Accepted{SHA: sha, HostAPI: declaredHostAPI(ctx, git, sha), Repository: repo}, nil
	}
	if err != nil {
		return scheduler.Accepted{}, rejected(scheduler.KindDeclaration, "%s", err.Error())
	}
	dirty, err := git.Dirty(ctx, inputPaths(loaded))
	if err != nil {
		return scheduler.Accepted{}, err
	}
	if len(dirty) > 0 {
		return scheduler.Accepted{}, &scheduler.Rejection{Kind: scheduler.KindDirty, Files: dirty,
			Message: fmt.Sprintf("commit first: %d uncommitted changes under the declared inputs: %s", len(dirty), strings.Join(dirty, ", "))}
	}
	suites := compose.Suites(loaded.Declaration, loaded.Tree)
	suite := in.Suite
	switch {
	case suite == "" && len(suites) == 1:
		suite = suites[0]
	case suite == "":
		return scheduler.Accepted{}, rejected(scheduler.KindDeclaration, "name a suite: %s holds %s", loaded.Declaration.Scripts.Suites, strings.Join(suites, ", "))
	case !slices.Contains(suites, suite):
		return scheduler.Accepted{}, rejected(scheduler.KindDeclaration, "commit %s has no suite %s in %s", sha[:12], suite, loaded.Declaration.Scripts.Suites)
	}
	return scheduler.Accepted{SHA: sha, HostAPI: loaded.Declaration.HostAPI, Repository: repo, Suite: suite}, nil
}

func inputPaths(l compose.Loaded) []string {
	out := make([]string, len(l.Declaration.Inputs.Dirty))
	for i, p := range l.Declaration.Inputs.Dirty {
		out[i] = string(p)
	}
	return out
}

func declaredHostAPI(ctx context.Context, git compose.Git, sha string) int {
	tree, err := git.ListTree(ctx, sha)
	if err != nil {
		return 0
	}
	raw, err := git.Blob(ctx, tree.Entries[compose.DeclarationPath].ID)
	if err != nil {
		return 0
	}
	var doc struct {
		HostAPI int `yaml:"hostApi"`
	}
	if yamlUnmarshal(raw, &doc) != nil {
		return 0
	}
	return doc.HostAPI
}
