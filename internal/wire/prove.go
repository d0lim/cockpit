package wire

import (
	"bytes"
	"encoding/json"
	"time"

	"github.com/d0lim/cockpit/internal/mask"
)

func Prove(record *mask.Masker, c Call) (Call, bool) {
	clean := Clean(record, c)
	if !clean.Withheld && proven(record, clean) {
		return clean, true
	}
	withheld := stub(record, clean)
	if !proven(record, withheld) {
		withheld.Caller, withheld.Target, withheld.Method = marker, marker, marker
	}
	return withheld, false
}

func proven(record *mask.Masker, c Call) bool {
	line, err := json.Marshal(Line{Call: &c})
	if err != nil {
		return false
	}
	again := Clean(record, c)
	second, err := json.Marshal(Line{Call: &again})
	if err != nil || !bytes.Equal(line, second) {
		return false
	}
	scanned, err := json.Marshal(Line{Call: new(withoutRecorderNumbers(c))})
	if err != nil {
		return false
	}
	return record.ValueScan().Mask(string(scanned)) == string(scanned)
}

func withoutRecorderNumbers(c Call) Call {
	c.Seq = 0
	c.Started, c.Replied, c.Finished = time.Time{}, time.Time{}, time.Time{}
	c.Request.Size, c.Response.Size = 0, 0
	return c
}
