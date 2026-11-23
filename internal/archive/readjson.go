package archive

import (
	"bytes"
	"encoding/json"
	"strings"

	"github.com/d0lim/cockpit/internal/contract"
)

func (r *reader) decodedClean(rel string, kind Kind, data []byte) (bool, error) {
	switch {
	case kind.Name == contract.ArchiveKindWireCalls:
		return true, nil
	case strings.HasSuffix(rel, ".jsonl"):
		for _, line := range bytes.Split(data, []byte("\n")) {
			if len(bytes.TrimSpace(line)) == 0 {
				continue
			}
			clean, err := r.cleanJSON(rel, line)
			if err != nil || !clean {
				return clean, err
			}
		}
		return true, nil
	case strings.HasSuffix(rel, ".json"):
		return r.cleanJSON(rel, data)
	}
	return true, nil
}

func (r *reader) cleanJSON(rel string, raw []byte) (bool, error) {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false, &Invalid{Reason: "a JSON file of the bundle does not parse", Files: []string{r.m.Mask(DirReport + "/" + rel)}, cause: err}
	}
	var decoded bytes.Buffer
	enc := json.NewEncoder(&decoded)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(v); err != nil {
		return false, err
	}
	text := decoded.String()
	return r.m.Mask(text) == text, nil
}
