package store

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var credentialFile = regexp.MustCompile(`(?:^|/)(?:\.env(?:\.[^/]*)?|[^/]*\.private\.env\.json|[^/]*\.pem)$`)

var whitespace = regexp.MustCompile(`\s`)

func CredentialPathOnly(value string, extra ...*regexp.Regexp) bool {
	if whitespace.MatchString(value) {
		return false
	}
	if credentialFile.MatchString(value) {
		return true
	}
	for _, re := range extra {
		if re.MatchString(value) {
			return true
		}
	}
	return false
}

func credentialPathAt(m Meta, extra []*regexp.Regexp) string {
	data, err := json.Marshal(m)
	if err != nil {
		return "meta"
	}
	var tree any
	if json.Unmarshal(data, &tree) != nil {
		return "meta"
	}
	return walkCredential(tree, "meta", extra)
}

func walkCredential(value any, at string, extra []*regexp.Regexp) string {
	switch v := value.(type) {
	case string:
		if CredentialPathOnly(v, extra...) {
			return at
		}
	case []any:
		for i, item := range v {
			if hit := walkCredential(item, fmt.Sprintf("%s[%d]", at, i), extra); hit != "" {
				return hit
			}
		}
	case map[string]any:
		keys := make([]string, 0, len(v))
		for k := range v {
			keys = append(keys, k)
		}
		sort.Strings(keys)
		for _, k := range keys {
			if hit := walkCredential(v[k], strings.Join([]string{at, k}, "."), extra); hit != "" {
				return hit
			}
		}
	}
	return ""
}
