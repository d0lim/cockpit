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

func Test드라이버_진입점은_루트_안_파일을_상대_경로로_끌어와_함수로_감싼다(t *testing.T) {
	root := writeTree(t, map[string]string{
		"suites/main.ts": "export default { name: \"joint\", flows: [1, 2] };\nexport const suite = undefined;\n",
	})
	p := newProbe(SiteRunner)
	bundle, err := driverBundle(t, root, "import * as mod from \"./suites/main.ts\";\nimport { record } from \"cockpit:flow\";\nexport default () => record(JSON.stringify(mod.default));\n", SiteRunner)
	if err != nil {
		t.Fatalf("번들: %v", err)
	}
	if err := newEngine(t, SiteRunner, p, 5*time.Second).Run(context.Background(), bundle, "default"); err != nil {
		t.Fatalf("실행: %v", err)
	}
	if !slices.Equal(p.records, []string{`{"name":"joint","flows":[1,2]}`}) {
		t.Fatalf("기록 %q", p.records)
	}
}

func Test드라이버가_루트_밖을_끌어오면_번들이_거부된다(t *testing.T) {
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

func Test프레임은_스크립트_파일과_줄을_안쪽부터_돌려주고_번들_밖_프레임은_뺀다(t *testing.T) {
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
		t.Fatalf("실행: %v", err)
	}
	if len(frames) < 2 || frames[0] != (Frame{File: "flows/a.ts", Line: 3}) || frames[1] != (Frame{File: "suites/main.ts", Line: 5}) {
		t.Fatalf("프레임 %+v", frames)
	}
	for _, f := range frames {
		if f.File == "" || f.File == bundleName || f.Line < 1 {
			t.Fatalf("번들 밖 프레임이 섞였다: %+v", frames)
		}
	}
	raw, _ := json.Marshal(frames[0])
	if string(raw) != `{"file":"flows/a.ts","line":3}` {
		t.Fatalf("직렬화 %s", raw)
	}
}
