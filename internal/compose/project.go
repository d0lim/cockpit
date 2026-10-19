package compose

import (
	"context"
	"encoding/json"
	"fmt"
	"slices"
	"strings"
)

const failureTail = 20

type Project struct {
	Docker Docker
	Name   string
	Args   []string
	Mask   func(string) string
	Log    func(line string)
}

type PsRow struct {
	Service  string `json:"Service"`
	Name     string `json:"Name"`
	ID       string `json:"ID"`
	State    string `json:"State"`
	Health   string `json:"Health"`
	ExitCode int    `json:"ExitCode"`
	Labels   string `json:"Labels"`
}

func (r PsRow) OneOff() bool {
	return slices.Contains(strings.Split(r.Labels, ","), "com.docker.compose.oneoff=True")
}

type Failed struct {
	Step string
	Code int
	Tail string
}

func (f *Failed) Error() string {
	return fmt.Sprintf("%s failed (exit code %d): %s", f.Step, f.Code, f.Tail)
}

func (p Project) run(ctx context.Context, step string, args []string) (Result, error) {
	res, err := p.Docker.Run(ctx, Command{Args: args})
	if p.Log != nil {
		for _, line := range strings.Split(string(res.Stderr), "\n") {
			if strings.TrimSpace(line) != "" {
				p.Log(p.Mask(line))
			}
		}
	}
	if err != nil {
		return res, fmt.Errorf("%s: %w", step, err)
	}
	if res.Code != 0 {
		return res, &Failed{Step: step, Code: res.Code, Tail: p.Mask(tail(string(res.Stderr), failureTail))}
	}
	return res, nil
}

func (p Project) Compose(ctx context.Context, step string, sub ...string) (Result, error) {
	return p.run(ctx, step, append(slices.Clone(p.Args), sub...))
}

func (p Project) ByName(ctx context.Context, step string, sub ...string) (Result, error) {
	return p.run(ctx, step, append([]string{"compose", "--ansi", "never", "-p", p.Name}, sub...))
}

func (p Project) Config(ctx context.Context) (map[string]any, error) {
	res, err := p.Compose(ctx, "compose config", "--profile", "*", "config", "--format", "json")
	if err != nil {
		return nil, err
	}
	return decodeConfig(res.Stdout)
}

func decodeConfig(data []byte) (map[string]any, error) {
	var doc map[string]any
	if err := json.Unmarshal(data, &doc); err != nil {
		return nil, fmt.Errorf("compose config did not print a JSON object")
	}
	return doc, nil
}

func (p Project) RunService(ctx context.Context, profile, service string, args ...string) (Result, error) {
	sub := append([]string{"--profile", profile, "run", "--rm", "--no-deps", "-T", service}, args...)
	res, err := p.Docker.Run(ctx, Command{Args: append(slices.Clone(p.Args), sub...)})
	if err != nil {
		return res, fmt.Errorf("compose run %s: %w", service, err)
	}
	return res, nil
}

func (p Project) Build(ctx context.Context, builder string) error {
	if builder != "" {
		if _, err := p.Compose(ctx, "builder "+builder, "--profile", "*", "run", "--rm", "--no-deps", "-T", builder); err != nil {
			return err
		}
	}
	_, err := p.Compose(ctx, "compose build", "build")
	return err
}

func (p Project) Up(ctx context.Context) error {
	_, err := p.Compose(ctx, "compose up", "up", "-d", "--no-build")
	return err
}

func (p Project) Ps(ctx context.Context) ([]PsRow, error) {
	res, err := p.ByName(ctx, "compose ps", "ps", "-a", "--format", "json")
	if err != nil {
		return nil, err
	}
	return ParsePs(res.Stdout)
}

func ParsePs(data []byte) ([]PsRow, error) {
	raws, err := ParseJSONRows(data)
	if err != nil {
		return nil, err
	}
	rows := make([]PsRow, 0, len(raws))
	for _, raw := range raws {
		var row PsRow
		if err := json.Unmarshal(raw, &row); err != nil {
			return nil, fmt.Errorf("cannot read a compose ps row: %w", err)
		}
		rows = append(rows, row)
	}
	return rows, nil
}

func (p Project) Down(ctx context.Context) error {
	if _, err := p.RemoveOneOffs(ctx, ""); err != nil {
		return err
	}
	_, err := p.ByName(ctx, "compose down", "--profile", "*", "down", "-v", "--remove-orphans")
	return err
}

func (p Project) Leftovers(ctx context.Context) (int, error) {
	total := 0
	for _, kind := range [][]string{{"ps", "-aq"}, {"network", "ls", "-q"}, {"volume", "ls", "-q"}} {
		res, err := p.run(ctx, "docker "+kind[0], append(slices.Clone(kind), "--filter", "label=com.docker.compose.project="+p.Name))
		if err != nil {
			return 0, err
		}
		total += len(strings.Fields(string(res.Stdout)))
	}
	return total, nil
}
