package totp

import (
	"crypto/hmac"
	"crypto/sha1"
	"encoding/binary"
	"errors"
	"fmt"
	"strings"
	"unicode"
)

const (
	StepMillis       int64 = 30_000
	waitMarginMillis int64 = 300
	alphabet               = "ABCDEFGHIJKLMNOPQRSTUVWXYZ234567"
	digits                 = 6
)

func DecodeBase32(input string) ([]byte, error) {
	cleaned := strings.Map(func(r rune) rune {
		if r == '=' || unicode.IsSpace(r) {
			return -1
		}
		return unicode.ToUpper(r)
	}, input)
	var out []byte
	buf, bits := 0, 0
	for _, ch := range cleaned {
		v := strings.IndexRune(alphabet, ch)
		if v < 0 {
			return nil, errors.New("input has a character outside base32")
		}
		buf = ((buf << 5) | v) & 0xffff
		bits += 5
		if bits >= 8 {
			bits -= 8
			out = append(out, byte((buf>>bits)&0xff))
		}
	}
	return out, nil
}

func At(seedBase32 string, step int64) (string, error) {
	if seedBase32 == "" {
		return "", errors.New("two-factor seed is empty")
	}
	key, err := DecodeBase32(seedBase32)
	if err != nil {
		return "", err
	}
	var counter [8]byte
	binary.BigEndian.PutUint64(counter[:], uint64(step))
	mac := hmac.New(sha1.New, key)
	mac.Write(counter[:])
	sum := mac.Sum(nil)
	offset := sum[len(sum)-1] & 0x0f
	value := binary.BigEndian.Uint32(sum[offset:offset+4]) & 0x7fffffff
	return fmt.Sprintf("%0*d", digits, value%1_000_000), nil
}

func ChooseStep(nowMs int64, last int64, hasLast bool) (step int64, waitMs int64) {
	now := nowMs / StepMillis
	step = now
	if hasLast && last+1 > now {
		step = last + 1
	}
	if step > now+1 {
		waitMs = (step-1)*StepMillis - nowMs + waitMarginMillis
	}
	return step, waitMs
}
