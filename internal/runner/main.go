package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"maps"
	"regexp"
	"slices"
	"strings"
	"sync/atomic"
	"time"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
	"github.com/d0lim/cockpit/internal/mask"
	"github.com/d0lim/cockpit/internal/ports/capture"
	"github.com/d0lim/cockpit/internal/ports/clock"
	cryptoport "github.com/d0lim/cockpit/internal/ports/crypto"
	httpport "github.com/d0lim/cockpit/internal/ports/http"
	"github.com/d0lim/cockpit/internal/ports/runnerctx"
	"github.com/d0lim/cockpit/internal/ports/source"
	"github.com/d0lim/cockpit/internal/ports/totp"
	"github.com/d0lim/cockpit/internal/version"
)

const (
	requestTimeout = 30 * time.Second
	maxBody        = 8 << 20
	maxSource      = 4 << 20
	runDeadline    = 24 * time.Hour
	rejected       = "거절: "
)

var suiteNamePattern = regexp.MustCompile(`^[a-z][a-z0-9-]{0,40}$`)

type Deps struct {
	Stdout io.Writer
	Stderr io.Writer
	Dial   httpport.Dialer
	Clock  clock.Clock
}

func parseArgs(args []string) (string, []string, error) {
	if len(args) == 0 || strings.HasPrefix(args[0], "-") {
		return "", nil, errors.New("쓰는 법: cockpit-runner run <스위트> [--flow <플로우 id>]...")
	}
	name := args[0]
	if !suiteNamePattern.MatchString(name) {
		return "", nil, fmt.Errorf("스위트 이름 %q 이 %s 꼴이 아니다", name, suiteNamePattern)
	}
	var targets []string
	rest := args[1:]
	for i := 0; i < len(rest); i++ {
		if rest[i] != "--flow" || i+1 >= len(rest) {
			return "", nil, fmt.Errorf("모르는 인자: %s", rest[i])
		}
		id := rest[i+1]
		if !flowIDPattern.MatchString(id) {
			return "", nil, fmt.Errorf("플로우 id %q 가 %s 꼴이 아니다", id, flowIDPattern)
		}
		targets = append(targets, id)
		i++
	}
	return name, targets, nil
}

func driverSource(name string) string {
	return "import * as mod from \"./suites/" + name + ".ts\";\nimport { __drive } from \"cockpit:flow\";\nexport default () => __drive(mod);\n"
}

func Main(ctx context.Context, args []string, getenv func(string) string, deps Deps) int {
	name, targets, err := parseArgs(args)
	if err != nil {
		fmt.Fprintln(deps.Stderr, err)
		return 2
	}
	s := loadSettings(getenv)
	clk := deps.Clock
	if clk.Now == nil {
		clk = clock.System()
	}
	base, err := mask.New(mask.Options{Mode: mask.Runner})
	if err != nil {
		fmt.Fprintln(deps.Stderr, "러너 마스커를 만들지 못했다")
		return 2
	}
	var current atomic.Pointer[mask.Masker]
	current.Store(base)
	maskText := func(t string) string { return current.Load().Mask(t) }
	fail := func(text string) int {
		fmt.Fprintln(deps.Stderr, maskText(text))
		return 2
	}
	hostAPI, err := declaredHostAPI(s.declaration)
	if err != nil {
		return fail(err.Error())
	}
	if hostAPI != contract.HostAPI {
		return fail(fmt.Sprintf("선언의 hostApi %d 가 러너의 hostApi %d 와 다르다", hostAPI, contract.HostAPI))
	}
	priv, err := totp.ReadPrivateEnv(s.privPath)
	if err != nil {
		return fail("private env " + s.privPath + " 을 읽지 못했다: " + err.Error())
	}
	current.Store(base.WithKnown(slices.Sorted(maps.Values(priv))))
	publicEnv, err := loadPublicEnv(s.envPath)
	if err != nil {
		return fail(err.Error())
	}
	ledger, err := ReadLedger(s.dir)
	if err != nil {
		return fail("원장을 읽지 못했다: " + err.Error())
	}
	e2eCtx, err := capture.Load(s.dir)
	if err != nil {
		return fail("컨텍스트를 읽지 못했다: " + err.Error())
	}
	roots, err := sourceRoots(s.sources)
	if err != nil {
		return fail(err.Error())
	}
	reader, err := source.Open(roots, maxSource, maskText)
	if err != nil {
		return fail(err.Error())
	}
	defer reader.Close()
	client, err := httpport.New(httpport.Options{Allowed: allowedOrigins(publicEnv), Timeout: requestTimeout, MaxBody: maxBody, Mask: maskText, Dial: deps.Dial})
	if err != nil {
		return fail("공개 env 의 주소를 허용 목록으로 만들지 못했다: " + err.Error())
	}
	ctxDeps := &runnerctx.Deps{
		Env: publicEnv, Priv: priv, PrivPath: s.privPath, SeedKeys: seedKeys(publicEnv),
		HTTP: client, Context: e2eCtx, Source: reader, Clock: clk, Keys: cryptoport.NewKeys(maskText), Mask: maskText,
	}
	ctxDeps.Changed = func() { current.Store(base.WithKnown(ctxDeps.KnownValues())) }
	ctxDeps.Changed()
	var execOut func(string) string
	if len(targets) > 0 {
		execOut = ExecOutPath(s.dir, s.execOut, targets, clk.Now())
	}
	driver := NewDriver(DriverDeps{
		Dir: s.dir, Suite: name, Targets: targets,
		Out:   func(line string) { fmt.Fprintln(deps.Stdout, line) },
		Clock: clk, HTTP: client, Context: e2eCtx, Ledger: ledger, Masker: current.Load,
		Identity: contract.RunnerIdentity{Version: version.String(), Image: s.image},
		ExecOut:  execOut,
	})
	bundle, err := engine.BuildBundle(ctx, engine.BundleInput{Driver: driverSource(name), Root: s.scripts, Site: engine.SiteRunner})
	if err != nil {
		return fail("스위트 " + name + " 을 번들하지 못했다: " + err.Error())
	}
	eng, err := engine.New(engine.Options{
		Site:    engine.SiteRunner,
		Budget:  engine.Budget{Deadline: runDeadline},
		Modules: map[string]engine.ModuleBuilder{engine.ModuleFlow: Module(driver, ctxDeps)},
		Log:     slog.New(consoleHandler{w: deps.Stderr, mask: maskText}),
	})
	if err != nil {
		return fail("엔진을 만들지 못했다: " + err.Error())
	}
	if err := eng.Run(ctx, bundle, "default"); err != nil {
		var f *engine.Failure
		if errors.As(err, &f) && strings.HasPrefix(f.Message, rejected) {
			return fail(f.Message)
		}
		return fail("러너 내부 오류(" + name + "): " + err.Error())
	}
	if !driver.Done() {
		return fail("러너 내부 오류(" + name + "): 드라이버가 끝까지 돌지 않았다")
	}
	return driver.ExitCode()
}
