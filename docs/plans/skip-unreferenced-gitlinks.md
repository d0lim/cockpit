# Skip unreferenced gitlinks plan

Goal. A repository can pin a submodule that only some of its declarations use, and a run of another declaration neither needs that submodule initialized nor reads it. The hub splices a submodule into the run only when the run's declaration references it.

## The problem

`ListTree` spliced every gitlink of the submitted commit, recursively, before the hub read the declaration (`Tree.splice`). Every gitlink had to resolve, including one that no declared path touches. A repository that adds a private submodule for one declaration therefore made every run of every other declaration need that submodule, and the submodules nested in it, initialized in the main checkout. Each run also extracted every pinned submodule into `src/` and `build/`, whether the stack read it or not.

## Decisions

- A gitlink is spliced when one of the run's paths lies in it or contains it. The run's paths are the declaration file and every path the declaration names, the same table confinement checks (`declaredPaths`): `compose.files`, `compose.projectDirectory`, `inputs.dirty`, `inputs.runnerSources[].path`, `images[].inputs`, `env.outputs`, `secrets.sources[].path`, `scripts.dir` and the paths inside it. The rule applies again inside each spliced submodule, so a nested submodule is spliced only when a path reaches it. One table decides confinement and splicing, so a new path field cannot be confined without being spliced.
- Splicing happens in two steps. `ListTree` lists the commit and splices the gitlinks that the declaration file lies in, so the file reads even when `.cockpit` is a submodule. After the schema check, `Tree.Splice` splices the gitlinks that the declared paths reference. A referenced gitlink that does not resolve refuses the run as `submodule` with the main checkout hint, as before. The `submodule` check moves after `schema` and before `confinement`.
- The hub does not resolve an unreferenced gitlink: it reads neither its `.gitmodules` entry nor its module, and the gitlink does not count toward the limit of 64 submodules. `git archive` writes it as an empty directory, so the run tree holds it the way a checkout holds a submodule it never initialized.
- A compose include value that resolves at or inside an unspliced gitlink refuses the declaration as `compose` at the include's field, and the message names the gitlink. The rule runs in the model walk (`walk.confine`), which resolves every include value the way compose does, so it checks the paths compose would read. The fix is to name the submodule in a declared path, such as `inputs.dirty`.
- Image keys, the dirty check and the extraction keep their code. Each reads only declared paths and the model's files, and each of those lies in a spliced repository or refuses the run first. A submodule root as an image input still counts as its pinned commit id.
- Rejected: splicing a gitlink when an include first reaches it. That keeps working a consumer that includes a submodule's stack without declaring the submodule, but the set of submodules a run reads would then depend on the compose files, the dirty check would still skip the submodule, and a build context in it would still need a declared path. A rule that reads only the declaration is easier to predict.

## Checklist

- [ ] Docs first: `docs/harness.md` (the check order, the include rule, the Git section, the run directory), `docs/hub.md` (declared paths, submodules in section 7, the action, the refusal table), `docs/lifecycle.md` (acceptance) and `docs/sealing.md` (the pinned commit seal). Evidence: the first commit of this branch.
- [ ] Failing tests first, then code. Evidence: one commit with the tests, each seen failing before the code: unreferenced and referenced gitlinks, nested splicing, the declaration file in a submodule and the limit (`internal/compose/tree_test.go`), the include refusal (`internal/compose/includegitlink_test.go`), the hub accepting an umbrella whose unreferenced submodule is uninitialized (`internal/hub/pipeline/gitlink_test.go`).
- [ ] Mutations, each red in a scratch copy, recorded at the head of each test file.
- [ ] `make verify` on the committed head.
- [ ] Live with the head binary and a trunk binary on fixture repositories: no submodule, an unreferenced uninitialized gitlink, a referenced initialized gitlink before and after a new pin, a referenced uninitialized gitlink, an include into an unreferenced gitlink, a nested submodule, an edit inside a referenced submodule, the run tree of the unreferenced gitlink, and a submodule initialized only in the main checkout submitted from a linked worktree.
- [ ] CI green.

## Left open

- A build context, a bind source or `env.command` that reads an unspliced submodule sees an empty directory. The hub does not resolve build contexts or bind sources against the commit's tree, and the seal checks `paths.build` and `mounts.binds` only check that they lie inside the run directory. A stack that builds or binds from a submodule names it in a declared path, usually `images[].inputs`.
