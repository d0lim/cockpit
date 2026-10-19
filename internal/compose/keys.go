package compose

import (
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

const imageKeyLength = 12

func KeyInputs(decl contract.Declaration) map[string][]string {
	out := map[string][]string{}
	for _, img := range decl.Images {
		set := map[string]bool{}
		for _, f := range decl.Compose.Files {
			set[string(f)] = true
		}
		for _, p := range img.Inputs {
			set[string(p)] = true
		}
		out[string(img.Key)] = sortedKeys(set)
	}
	return out
}

func ImageKeys(decl contract.Declaration, tree Tree) (map[string]string, error) {
	keys := map[string]string{}
	for key, paths := range KeyInputs(decl) {
		ids := make([]string, 0, len(paths))
		for _, p := range paths {
			e, ok := tree.Entries[p]
			if !ok {
				return nil, fmt.Errorf("image key %s: input %s is not in commit %s", key, p, tree.SHA[:12])
			}
			ids = append(ids, e.ID)
		}
		keys[key] = keyOf(ids)
	}
	return keys, nil
}

func keyOf(ids []string) string {
	sorted := slices.Clone(ids)
	slices.Sort(sorted)
	sum := sha256.Sum256([]byte(strings.Join(sorted, "\n")))
	return hex.EncodeToString(sum[:])[:imageKeyLength]
}
