package engine

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/dop251/goja"
)

func driverBundle(t *testing.T, root, driver string, site Site) (Bundle, error) {
	t.Helper()
	return BuildBundle(context.Background(), BundleInput{Driver: driver, Root: root, Site: site})
}

func TestDriverEntryImportsRootFilesByRelativePathAndWrapsTheBundleInAFunction(t *testing.T) {
	root := writeTree(t, map[string]string{
		"suites/main.ts": "export default { name: \"joint\", flows: [1, 2] };\nexport const suite = undefined;\n",
	})
	p := newProbe(SiteRunner)
	bundle, err := driverBundle(t, root, "import * as mod from \"./suites/main.ts\";\nimport { record } from \"cockpit:flow\";\nexport default () => record(JSON.stringify(mod.default));\n", SiteRunner)
	if err != nil {
		t.Fatalf("bundle: %v", err)
	}
	if err := newEngine(t, SiteRunner, p, 5*time.Second).Run(context.Background(), bundle, "default"); err != nil {
		t.Fatalf("run: %v", err)
	}
	if !slices.Equal(p.records, []string{`{"name":"joint","flows":[1,2]}`}) {
		t.Fatalf("records %q", p.records)
	}
}

func TestDriverImportingFromOutsideTheRootIsRejectedAtBundleTime(t *testing.T) {
	outside := writeTree(t, map[string]string{"secret.ts": "export const secret = \"PLANTED\";\n"})
	root := writeTree(t, map[string]string{"suites/main.ts": "export default 1;\n"})
	_, err := driverBundle(t, root, "import { secret } from \""+outside+"/secret.ts\";\nexport default () => secret;\n", SiteRunner)
	var be *BundleError
	if !errors.As(err, &be) || !strings.Contains(err.Error(), "outside the boundary") {
		t.Fatalf("%v", err)
	}
	_, err = driverBundle(t, root, "import { secret } from \"../secret.ts\";\nexport default () => secret;\n", SiteRunner)
	if !errors.As(err, &be) {
		t.Fatalf("%v", err)
	}
}

func TestFramesListScriptFileAndLineInnermostFirstAndDropFramesOutsideTheBundle(t *testing.T) {
	root := writeTree(t, map[string]string{
		"flows/a.ts":     "import { where } from \"cockpit:flow\";\n\nexport const at = () => where();\n",
		"suites/main.ts": "import { at } from \"../flows/a\";\nimport { record } from \"cockpit:flow\";\n\nexport default () => {\n\trecord(JSON.stringify(at()));\n};\n",
	})
	p := newProbe(SiteRunner)
	var frames []Frame
	p.extra = map[string]any{"where": func(call goja.FunctionCall) goja.Value {
		frames = p.host.Frames()
		return goja.Undefined()
	}}
	err := newEngine(t, SiteRunner, p, 5*time.Second).Run(context.Background(), mustBundle(t, root, "suites/main.ts", SiteRunner), "default")
	if err != nil {
		t.Fatalf("run: %v", err)
	}
	if len(frames) < 2 || frames[0] != (Frame{File: "flows/a.ts", Line: 3}) || frames[1] != (Frame{File: "suites/main.ts", Line: 5}) {
		t.Fatalf("frames %+v", frames)
	}
	for _, f := range frames {
		if f.File == "" || f.File == bundleName || f.Line < 1 {
			t.Fatalf("frames outside the bundle leaked in: %+v", frames)
		}
	}
	raw, _ := json.Marshal(frames[0])
	if string(raw) != `{"file":"flows/a.ts","line":3}` {
		t.Fatalf("marshaled %s", raw)
	}
}
