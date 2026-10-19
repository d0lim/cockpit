package main

import (
	"fmt"
	"regexp"
	"slices"
	"strings"
)

var (
	topDecl    = regexp.MustCompile(`^\t(?:export )?(?:interface|type|function) ([A-Za-z0-9_]+)`)
	memberDecl = regexp.MustCompile(`^\t\t(?:readonly )?([A-Za-z0-9_]+)\??[(:<]`)
)

func withDocs(text string, docs map[string]string, used map[string]bool) string {
	var out []string
	owner := ""
	for _, line := range strings.Split(text, "\n") {
		key, indent := "", ""
		if m := topDecl.FindStringSubmatch(line); m != nil {
			key, indent = m[1], "\t"
			owner = ""
			if strings.HasSuffix(line, "{") {
				owner = m[1]
			}
		} else if m := memberDecl.FindStringSubmatch(line); m != nil && owner != "" {
			key, indent = owner+"."+m[1], "\t\t"
		} else if line == "\t}" {
			owner = ""
		}
		if doc, ok := docs[key]; ok && key != "" {
			out = append(out, indent+"/** "+doc+" */")
			used[key] = true
		}
		out = append(out, line)
	}
	return strings.Join(out, "\n")
}

func unusedDocs(docs map[string]string, used map[string]bool) error {
	var unused []string
	for key := range docs {
		if !used[key] {
			unused = append(unused, key)
		}
	}
	if len(unused) == 0 {
		return nil
	}
	slices.Sort(unused)
	return fmt.Errorf("doc keys match no declaration: %s", strings.Join(unused, ", "))
}
