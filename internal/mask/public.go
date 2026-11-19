package mask

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

type PublicField struct {
	Key   string
	Value string
}

type publicField struct {
	key   string
	value *regexp.Regexp
}

type publicProbe struct {
	name   string
	sample string
}

var (
	publicKeyFamily = regexp.MustCompile(`(?i)^(?:credential|[\w-]*token|session)$`)
	neverPublicKey  = regexp.MustCompile(`(?i)password|passwd|pwd|secret|api[_-]?key|authori[sz]ation|cookie`)
	publicProbes    = []publicProbe{
		{"the empty string", ""},
		{"16 lowercase letters", "qwertyuiopasdfgh"},
		{"16 capitals", "QWERTYUIOPASDFGH"},
		{"16 digits", "4929173650281746"},
		{"32 hex characters", "9f86d081884c7d659a2feaa0c55ad015"},
		{"43 base64url characters", "n4bQgYhMfWWaL-qgxVrQFaO_TxsrC4Is0V1sFbDwCgg"},
		{"a UUID", "123e4567-e89b-42d3-a456-426614174000"},
		{"a JWT", "eyJhbGciOiJIUzI1NiJ9.eyJzdWIiOiIxMjM0In0.c2lnbmF0dXJl"},
		{"a mixed password", "Tr0ub4dor&3-Xy!9"},
	}
)

func PublicFieldKey(key string) bool { return publicKeyFamily.MatchString(key) }

func CheckPublicField(f PublicField) error {
	_, err := compilePublicField(f)
	return err
}

func compilePublicField(f PublicField) (publicField, error) {
	if !publicKeyFamily.MatchString(f.Key) || neverPublicKey.MatchString(f.Key) {
		return publicField{}, fmt.Errorf("key %q is not a key of the token, session or credential family", f.Key)
	}
	if _, err := regexp.Compile(f.Value); err != nil {
		return publicField{}, fmt.Errorf("value of key %q is not RE2: %w", f.Key, err)
	}
	whole, err := regexp.Compile(`^(?:` + f.Value + `)$`)
	if err != nil {
		return publicField{}, fmt.Errorf("value of key %q is not RE2: %w", f.Key, err)
	}
	for _, p := range publicProbes {
		if whole.MatchString(p.sample) {
			return publicField{}, fmt.Errorf("value of key %q also matches %s, so it would expose real credentials", f.Key, p.name)
		}
	}
	return publicField{key: f.Key, value: whole}, nil
}

func (m *Masker) WithPublicFields(fields []PublicField) (*Masker, error) {
	compiled := make([]publicField, 0, len(fields))
	for _, f := range fields {
		c, err := compilePublicField(f)
		if err != nil {
			return nil, err
		}
		compiled = append(compiled, c)
	}
	next := *m
	next.public = compiled
	return &next, nil
}

func (m *Masker) ValueScan() *Masker {
	next := *m
	next.keys = nil
	next.public = nil
	next.rules = slices.DeleteFunc(slices.Clone(m.rules), func(r *rule) bool { return r.byKey })
	return &next
}

func (m *Masker) publicValue(key, value string) bool {
	return slices.ContainsFunc(m.public, func(f publicField) bool {
		return strings.EqualFold(f.key, key) && f.value.MatchString(value)
	})
}
