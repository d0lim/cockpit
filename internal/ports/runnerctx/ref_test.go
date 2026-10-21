package runnerctx

import (
	"slices"
	"strings"
	"testing"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/ports/porttest"
)

var refMutations = []porttest.Mutation{
	{"a ref'd capture is no longer masked", "keep every capture in KnownValues", "red: the ref'd id is still a known value", "TestARefMarksACapturedIdPublicAndRecordsItOnTheStep"},
	{"a private env value cannot be made public", "secretLike passes no private values to Secret", "red: ref of the planted value is accepted and the value leaves the known values", "TestARefOfAPrivateValueOrACredentialPatternIsRefusedAndStaysMasked"},
	{"without a Secret check every ref is refused", "secretLike answers false when Secret is nil", "red: a ref is accepted with no Secret check", "TestARefWithoutASecretCheckIsRefused"},
	{"a capture that no ref names stays masked", "treat every capture as public", "red: the plain capture leaves the known values", "TestARefMarksACapturedIdPublicAndRecordsItOnTheStep"},
	{"a step keeps at most 50 refs", "drop the maxRefs check", "red: the 51st ref is accepted", "TestRefNamesValuesAndCountAreBounded"},
	{"the same ref twice in one step records once", "drop the Contains check in addRef", "red: the transaction ref is recorded twice", "TestARefMarksACapturedIdPublicAndRecordsItOnTheStep"},
}

func TestRefMutationRecordMatchesTheTestsInThisFile(t *testing.T) {
	porttest.AssertMutationRecord(t, "ref_test.go", refMutations)
}

func refWorld(t *testing.T) *world {
	w := newWorld(t)
	w.deps.Secret = func(v string, priv []string) bool {
		for _, p := range priv {
			if len(p) >= 6 && strings.Contains(v, p) {
				return true
			}
		}
		return strings.Contains(v, "tok_live_abc")
	}
	return w
}

func TestARefMarksACapturedIdPublicAndRecordsItOnTheStep(t *testing.T) {
	w := refWorld(t)
	w.mustRun(`export default async () => {
	ctx.capture("transferId", "wvt-0001");
	ctx.capture("session", "sess-secret-9");
	ctx.ref("transaction", "wvt-0001");
	ctx.ref("transaction", "wvt-0001");
	ctx.ref("txHash", "wvt-0001");
}`)
	known := w.deps.KnownValues()
	if slices.Contains(known, "wvt-0001") || !slices.Contains(known, "sess-secret-9") || !slices.Contains(known, planted) {
		t.Fatalf("known values %v", known)
	}
	refs := w.deps.TakeRefs()
	if !slices.Equal(refs, []contract.StepRef{{Name: "transaction", Value: "wvt-0001"}, {Name: "txHash", Value: "wvt-0001"}}) || len(w.deps.TakeRefs()) != 0 {
		t.Fatalf("refs %v", refs)
	}
	if w.changed < 3 {
		t.Fatalf("the masker was not rebuilt after the ref: changed %d", w.changed)
	}
}

func TestARefOfAPrivateValueOrACredentialPatternIsRefusedAndStaysMasked(t *testing.T) {
	w := refWorld(t)
	w.mustRun(`export default async () => {
	ctx.capture("token", "tok_live_abc");
	must(catchCode(() => ctx.ref("password", ctx.priv.adminPassword)).startsWith("PORT_BAD_ARGS|"), "priv value");
	must(catchCode(() => ctx.ref("wrapped", "x-" + ctx.priv.adminPassword + "-y")).startsWith("PORT_BAD_ARGS|"), "value containing priv");
	must(catchCode(() => ctx.ref("token", "tok_live_abc")).startsWith("PORT_BAD_ARGS|"), "pattern");
	ctx.ref("transaction", "wvt-0002");
}`)
	known := w.deps.KnownValues()
	if !slices.Contains(known, planted) || !slices.Contains(known, "tok_live_abc") {
		t.Fatalf("a refused value left the known values: %v", known)
	}
	refs := w.deps.TakeRefs()
	if len(refs) != 1 || refs[0].Value != "wvt-0002" {
		t.Fatalf("canary: refs %v", refs)
	}
}

func TestRefNamesValuesAndCountAreBounded(t *testing.T) {
	w := refWorld(t)
	w.mustRun(`export default async () => {
	must(catchCode(() => ctx.ref("Bad", "x")).startsWith("PORT_BAD_ARGS|"), "upper-case name");
	must(catchCode(() => ctx.ref("a.b", "x")).startsWith("PORT_BAD_ARGS|"), "dotted name");
	must(catchCode(() => ctx.ref("empty", "")).startsWith("PORT_BAD_ARGS|"), "empty value");
	must(catchCode(() => ctx.ref("long", "x".repeat(201))).startsWith("PORT_BAD_ARGS|"), "long value");
	must(catchCode(() => ctx.ref("ctl", "a\nb")).startsWith("PORT_BAD_ARGS|"), "control character");
	must(catchCode(() => ctx.ref("num", 5 as any)).startsWith("PORT_BAD_ARGS|"), "value type");
	for (let i = 0; i < 50; i++) ctx.ref("id", "v-" + i);
	must(catchCode(() => ctx.ref("id", "v-50")).startsWith("PORT_LIMIT|"), "51st ref");
}`)
	if refs := w.deps.TakeRefs(); len(refs) != 50 || refs[49].Value != "v-49" {
		t.Fatalf("%d refs", len(refs))
	}
}

func TestARefWithoutASecretCheckIsRefused(t *testing.T) {
	w := newWorld(t)
	w.mustRun(`export default async () => {
	must(catchCode(() => ctx.ref("transaction", "wvt-0003")).startsWith("PORT_BAD_ARGS|"), "no secret check");
	ctx.capture("canary", "wvt-0003");
}`)
	if refs := w.deps.TakeRefs(); len(refs) != 0 || !slices.Contains(w.deps.KnownValues(), "wvt-0003") {
		t.Fatalf("refs %v", refs)
	}
}
