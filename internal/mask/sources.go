package mask

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"maps"
	"os"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"syscall"
	"unicode/utf8"
)

type Kind string

const (
	Dotenv Kind = "dotenv"
	JSON   Kind = "json"
	PEM    Kind = "pem"
)

type Source struct {
	Kind Kind
	Path string
}

type Sources struct {
	Dir         string
	Mode        Mode
	Outputs     []Source
	Declared    []Source
	PublicNames []string
}

type Values struct {
	Known   []string
	Missing []string
}

const maxSourceBytes = 1 << 20

var unresolved = regexp.MustCompile(`^\$\{[A-Za-z_][A-Za-z0-9_]*\}$`)

type entry struct {
	name, label, value string
}

type reader func(root *os.Root, rel string, max int64) (string, error)

func Refresh(src Sources) (Values, error) {
	return refresh(src, readRunFile)
}

func refresh(src Sources, read reader) (Values, error) {
	root, err := os.OpenRoot(src.Dir)
	if err != nil {
		return Values{}, fmt.Errorf("cannot open run directory: %w", err)
	}
	defer root.Close()
	var v Values
	seen := map[Source]bool{}
	for _, s := range slices.Concat(src.Outputs, src.Declared) {
		if seen[s] {
			continue
		}
		seen[s] = true
		text, err := read(root, s.Path, maxSourceBytes)
		if err != nil {
			return Values{}, err
		}
		entries, err := parse(s, text)
		if err != nil {
			return Values{}, err
		}
		for _, e := range entries {
			switch {
			case e.name != "" && slices.Contains(src.PublicNames, e.name):
			case e.value == "" || unresolved.MatchString(e.value):
				v.Missing = append(v.Missing, s.Path+"#"+e.label)
			case utf8.RuneCountInString(e.value) >= src.Mode.min():
				v.Known = append(v.Known, e.value)
			}
		}
	}
	return v, nil
}

func parse(s Source, text string) ([]entry, error) {
	switch s.Kind {
	case Dotenv:
		return dotenvEntries(text), nil
	case JSON:
		var doc any
		if json.Unmarshal([]byte(text), &doc) != nil {
			return nil, fmt.Errorf("cannot parse run credential file as JSON: %s", s.Path)
		}
		return jsonEntries(doc, "", ""), nil
	case PEM:
		return pemEntries(text), nil
	}
	return nil, fmt.Errorf("unknown source kind: %q (%s)", s.Kind, s.Path)
}

func dotenvEntries(text string) []entry {
	var order []string
	byName := map[string]string{}
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimFunc(raw, isJSSpace)
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		line = strings.TrimPrefix(line, "export ")
		eq := strings.IndexByte(line, '=')
		if eq <= 0 {
			continue
		}
		name := strings.TrimFunc(line[:eq], isJSSpace)
		value := strings.TrimFunc(line[eq+1:], isJSSpace)
		if len(value) >= 2 && (value[0] == '"' || value[0] == '\'') && value[len(value)-1] == value[0] {
			value = value[1 : len(value)-1]
		}
		if _, ok := byName[name]; !ok {
			order = append(order, name)
		}
		byName[name] = value
	}
	out := make([]entry, 0, len(order))
	for _, name := range order {
		out = append(out, entry{name: name, label: name, value: byName[name]})
	}
	return out
}

func JSONStrings(v any) []string {
	var out []string
	for _, e := range jsonEntries(v, "", "") {
		out = append(out, e.value)
	}
	return out
}

func jsonEntries(v any, name, label string) []entry {
	var out []entry
	switch x := v.(type) {
	case string:
		out = append(out, entry{name: name, label: label, value: x})
	case []any:
		for i, e := range x {
			out = append(out, jsonEntries(e, name, label+"["+strconv.Itoa(i)+"]")...)
		}
	case map[string]any:
		for _, k := range slices.Sorted(maps.Keys(x)) {
			out = append(out, jsonEntries(x[k], k, strings.TrimPrefix(label+"."+k, "."))...)
		}
	}
	return out
}

func pemEntries(text string) []entry {
	var out []entry
	inside := false
	for _, raw := range strings.Split(text, "\n") {
		line := strings.TrimFunc(raw, isJSSpace)
		switch {
		case strings.HasPrefix(line, "-----BEGIN "):
			inside = true
		case strings.HasPrefix(line, "-----END "):
			inside = false
		case inside && line != "":
			out = append(out, entry{label: "pem", value: line})
		}
	}
	return out
}

func readRunFile(root *os.Root, rel string, max int64) (string, error) {
	if !filepath.IsLocal(rel) {
		return "", fmt.Errorf("not a relative path inside the run directory: %s", rel)
	}
	rel = filepath.Clean(rel)
	info, err := root.Lstat(rel)
	if err != nil {
		return "", sourceError(rel, err)
	}
	if !info.Mode().IsRegular() {
		return "", fmt.Errorf("not a regular file: %s", rel)
	}
	f, err := openNoFollow(root, rel)
	if err != nil {
		return "", sourceError(rel, err)
	}
	defer f.Close()
	raw, err := io.ReadAll(io.LimitReader(f, max+1))
	switch {
	case err != nil:
		return "", fmt.Errorf("cannot read run credential file: %s", rel)
	case int64(len(raw)) > max:
		return "", fmt.Errorf("exceeds the size limit (%s): %s", sizeText(max), rel)
	}
	return string(raw), nil
}

func sourceError(rel string, err error) error {
	var errno syscall.Errno
	switch {
	case errors.Is(err, fs.ErrNotExist):
		return fmt.Errorf("run credential file not found: %s", rel)
	case errors.Is(err, errNotRegular), errors.Is(err, syscall.ELOOP):
		return fmt.Errorf("not a regular file: %s", rel)
	case errors.As(err, &errno):
		return fmt.Errorf("cannot read run credential file: %s: %w", rel, errno)
	}
	return fmt.Errorf("resolved path is outside the run directory: %s", rel)
}

func sizeText(n int64) string {
	const mib = 1 << 20
	if n%mib == 0 {
		return fmt.Sprintf("%d MiB", n/mib)
	}
	return fmt.Sprintf("%d bytes", n)
}

func isJSSpace(r rune) bool {
	switch r {
	case '\t', '\n', '\v', '\f', '\r', ' ', 0xa0, 0x1680, 0x2028, 0x2029, 0x202f, 0x205f, 0x3000, 0xfeff:
		return true
	}
	return r >= 0x2000 && r <= 0x200a
}

func DotenvValues(text string) map[string]string {
	out := map[string]string{}
	for _, e := range dotenvEntries(text) {
		out[e.name] = e.value
	}
	return out
}
