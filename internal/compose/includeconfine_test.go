package compose

import (
	"maps"
	"slices"
	"testing"

	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var includeConfineMutations = []porttest.Mutation{
	{"an include value with interpolation is rejected", "drop the $ check in walk.confine", "red: ${VAR}/x.yaml, which the commit holds, passes", "TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass"},
	{"an include value with a colon (remote include) is rejected", "drop the colon check in walk.confine", "red: oci:/registry/x.yaml and git@github.com:o/r.git, which the commit holds, pass", "TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass"},
	{"an include value starting with ~ is rejected", "drop the ~ check in walk.confine", "red: project_directory ~/pd, which the commit holds, passes", "TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass"},
	{"an absolute include value is rejected", "drop the path.IsAbs check in walk.confine", "red: /etc/x.yaml from stack/ passes as stack/etc/x.yaml", "TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass"},
	{"an include value stays inside the commit", "drop the cleanRelative check in walk.confine", "red: ../outside.yaml, .. and ../outside.env are rejected as missing, not as outside the commit", "TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass"},
	{"an include value passes through no committed link", "drop the tree.Link check in walk.confine", "red: the committed links linked.env and stack/.env pass", "TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass"},
	{"an include env_file is a committed file or an env output", "drop the file and env.outputs check for includeEnvFile in walk.confine", "red: env_file missing.env passes", "TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass"},
	{"the .env an included project reads is not a committed link", "skip the includeDotenv confinement in walk.include", "red: the committed link stack/.env passes", "TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass"},
	{"an include item holds only path, project_directory and env_file", "ignore other keys in parseItem", "red: the unknown key profiles passes", "TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass"},
	{"an included file named by compose.forbidden rejects the declaration", "drop the compose.forbidden check in walk.confine", "red: stack/Relay-TUNNEL.yaml is included and the declaration passes", "TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass"},
}

func TestIncludeConfineMutationRecordMatchesTestsInThisFileBothWays(t *testing.T) {
	porttest.AssertMutationRecord(t, "includeconfine_test.go", includeConfineMutations)
}

func stacked(include string, files map[string]string) map[string]string {
	out := map[string]string{"stack/compose.yaml": include + queueStack, "stack/run.env": "QUEUE=1\n"}
	for k, v := range files {
		out[k] = v
	}
	return out
}

var deniedIncludes = []includeCase{
	{name: "a parent segment out of the commit", include: "include:\n  - ../outside.yaml\n", field: "compose.files[0] include[0]", says: `resolves to "../outside.yaml", outside the commit`},
	{name: "an absolute path that the commit holds under the including directory", include: "include:\n  - stack/compose.yaml\n",
		files: stacked("include:\n  - /etc/x.yaml\n", map[string]string{"stack/etc/x.yaml": "services: {}\n"}), field: "compose.files[0] include[0] include[0]", says: `"/etc/x.yaml" is an absolute path`},
	{name: "an oci include that the commit holds as a path", include: "include:\n  - oci://registry/x.yaml\n", files: map[string]string{"oci:/registry/x.yaml": "services: {}\n"},
		field: "compose.files[0] include[0]", says: "holds a colon"},
	{name: "a git include that the commit holds as a path", include: "include:\n  - git@github.com:o/r.git\n", files: map[string]string{"git@github.com:o/r.git": "services: {}\n"},
		field: "compose.files[0] include[0]", says: "holds a colon"},
	{name: "interpolation that the commit holds as a path", include: "include:\n  - ${VAR}/x.yaml\n", files: map[string]string{"${VAR}/x.yaml": "services: {}\n"},
		field: "compose.files[0] include[0]", says: "uses interpolation"},
	{name: "a home project directory that the commit holds", include: "include:\n  - {path: stack/compose.yaml, project_directory: \"~/pd\"}\n", files: stacked("", map[string]string{"~/pd/keep": "x\n"}),
		field: "compose.files[0] include[0].project_directory", says: "starts with ~"},
	{name: "a project directory out of the commit", include: "include:\n  - {path: stack/compose.yaml, project_directory: ..}\n", files: stacked("", nil),
		field: "compose.files[0] include[0].project_directory", says: "outside the commit"},
	{name: "an env file out of the commit", include: "include:\n  - {path: stack/compose.yaml, env_file: ../outside.env}\n", files: stacked("", nil),
		field: "compose.files[0] include[0].env_file", says: "outside the commit"},
	{name: "an env file neither committed nor generated", include: "include:\n  - {path: stack/compose.yaml, env_file: [stack/run.env, missing.env]}\n", files: stacked("", nil),
		field: "compose.files[0] include[0].env_file[1]", says: "neither a file in the commit nor one of env.outputs"},
	{name: "an empty path", include: "include:\n  - \"\"\n", field: "compose.files[0] include[0]", says: "empty"},
	{name: "a committed link as a path", include: "include:\n  - {path: [stack/compose.yaml, linked.yaml]}\n", files: stacked("", nil), links: map[string]string{"linked.yaml": "stack/compose.yaml"},
		field: "compose.files[0] include[0].path[1]", says: "symbolic link linked.yaml"},
	{name: "a path through a committed directory link", include: "include:\n  - linkdir/compose.yaml\n", files: stacked("", nil), links: map[string]string{"linkdir": "stack"},
		field: "compose.files[0] include[0]", says: "symbolic link linkdir"},
	{name: "a committed link as an env file", include: "include:\n  - {path: stack/compose.yaml, env_file: linked.env}\n", files: stacked("", nil), links: map[string]string{"linked.env": "stack/run.env"},
		field: "compose.files[0] include[0].env_file", says: "symbolic link linked.env"},
	{name: "a committed link as the .env of the included project", include: "include:\n  - stack/compose.yaml\n", files: stacked("", nil), links: map[string]string{"stack/.env": "run.env"},
		field: "compose.files[0] include[0]", says: "symbolic link stack/.env"},
	{name: "a project directory that is a file", include: "include:\n  - {path: stack/compose.yaml, project_directory: stack/compose.yaml}\n", files: stacked("", nil),
		field: "compose.files[0] include[0].project_directory", says: "not a directory"},
	{name: "a path that is a directory", include: "include:\n  - stack\n", files: stacked("", nil), field: "compose.files[0] include[0]", says: "not a file"},
	{name: "an unknown include key", include: "include:\n  - {path: stack/compose.yaml, profiles: [x]}\n", files: stacked("", nil),
		field: "compose.files[0] include[0].profiles", says: `"profiles" is not an include key`},
	{name: "an item without a path", include: "include:\n  - {project_directory: stack}\n", files: stacked("", nil), field: "compose.files[0] include[0]", says: "names no path"},
	{name: "an include that is not a list", include: "include: stack/compose.yaml\n", files: stacked("", nil), field: "compose.files[0] include", says: "not a list"},
	{name: "a path that is a mapping", include: "include:\n  - {path: {file: x}}\n", field: "compose.files[0] include[0].path", says: "not a string"},
	{name: "an included file named by compose.forbidden", include: "include:\n  - stack/compose.yaml\n", files: stacked("include:\n  - Relay-TUNNEL.yaml\n", map[string]string{"stack/Relay-TUNNEL.yaml": "services: {}\n"}),
		field: "compose.files[0] include[0] include[0]", says: "compose.forbidden"},
}

var allowedIncludes = []includeCase{
	{name: "a path string", include: "include:\n  - stack/compose.yaml\n", files: stacked("", nil), field: "stack/compose.yaml"},
	{name: "every key with an env output and a committed env file", include: "include:\n  - {path: [stack/compose.yaml], project_directory: stack, env_file: [.env, stack/run.env]}\n", files: stacked("", nil), field: "stack/compose.yaml"},
	{name: "the commit root as project directory", include: "include:\n  - {path: stack/compose.yaml, project_directory: .}\n", files: stacked("", nil), field: "stack/compose.yaml"},
	{name: "a parent segment that stays inside", include: "include:\n  - stack/compose.yaml\n", files: stacked("include:\n  - ../shared/compose.yaml\n", map[string]string{"shared/compose.yaml": "services: {}\n"}), field: "shared/compose.yaml"},
	{name: "a committed .env file in the included project", include: "include:\n  - stack/compose.yaml\n", files: stacked("", map[string]string{"stack/.env": "QUEUE=1\n"}), field: "stack/compose.yaml"},
	{name: "an included file under a name compose.forbidden does not hold", include: "include:\n  - stack/compose.yaml\n", files: stacked("include:\n  - relay.yaml\n", map[string]string{"stack/relay.yaml": "services: {}\n"}), field: "stack/relay.yaml"},
	{name: "a committed .env link that env_file replaces", include: "include:\n  - {path: stack/compose.yaml, env_file: stack/run.env}\n", files: stacked("", nil), links: map[string]string{"stack/.env": "run.env"}, field: "stack/compose.yaml"},
}

func TestIncludeValuesThatLeaveTheCommitOrCannotBeResolvedAreRejectedAtTheirFieldAndInsideValuesPass(t *testing.T) {
	passed := 0
	for _, c := range allowedIncludes {
		loaded, err := loadToy(t, c.mutate(t))
		if err != nil || !slices.Contains(loaded.Model.Files, c.field) {
			t.Errorf("%s: files %q, %v", c.name, loaded.Model.Files, err)
			continue
		}
		for _, name := range append(slices.Collect(maps.Keys(c.files)), slices.Collect(maps.Keys(c.links))...) {
			if !loaded.Tree.Has(name) {
				t.Errorf("%s: canary: %s is not in the commit", c.name, name)
			}
		}
		passed++
	}
	if passed != 7 {
		t.Fatalf("%d of 7 allowed includes passed", passed)
	}
	denied := 0
	for _, c := range deniedIncludes {
		_, err := loadToy(t, c.mutate(t))
		includeRejected(t, c, err)
		denied++
	}
	if denied != 21 {
		t.Fatalf("%d denied includes ran, want 21", denied)
	}
}
