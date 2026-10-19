package store

import (
	"encoding/json"
	"fmt"
	"regexp"
	"sort"
	"strings"
)

var credentialFile = regexp.MustCompile(`(?:^|/)(?:\.env(?:\.[^/]*)?|[^/]*\.private\.env\.json|secrets\.ya?ml|[^/]*\.pem)$`)

var whitespace = regexp.MustCompile(`\s`)

func CredentialPathOnly(value string) bool {
	return !whitespace.MatchString(value) && credentialFile.MatchString(value)
}

func credentialPathAt(m Meta) string {
	data, err := json.Marshal(m)
	if err != nil {
		return "meta"
	}
	var tree any
	if json.Unmarshal(data, &tree) != nil {
		return "meta"
	}
	return walkCredential(tree, "meta")
}

func walkCredential(value any, at string) string {
	switch v := value.(type) {
	case string:
		if CredentialPathOnly(v) {
			return at
		}
	case []any:
		for i, item := range v {
			if hit := walkCredential(item, fmt.Sprintf("%s[%d]", at, i)); hit != "" {
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
			if hit := walkCredential(v[k], strings.Join([]string{at, k}, ".")); hit != "" {
				return hit
			}
		}
	}
	return ""
}
