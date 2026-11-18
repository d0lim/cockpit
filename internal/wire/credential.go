package wire

import (
	"regexp"
	"slices"
	"strings"

	"github.com/d0lim/cockpit/internal/mask"
)

var (
	credentialWords = []string{
		"password", "passwd", "passphrase", "pwd", "secret", "credential",
		"api_key", "api-key", "apikey", "private_key", "private-key", "privatekey",
		"authorization", "authorisation", "cookie", "bearer",
		"session_id", "session-id", "sessionid",
	}
	otpWord = regexp.MustCompile(`(?:^|[^A-Za-z0-9])(?i:t?otp)$|[a-z0-9](?:Otp|OTP|Totp|TOTP)$`)
)

func namesCredential(key string) bool {
	if mask.PublicFieldKey(key) {
		return false
	}
	segments := strings.Split(key, ".")
	return slices.ContainsFunc(segments, func(segment string) bool {
		return segmentNamesCredential(segment) || (len(segments) > 1 && mask.SecretKeyName(segment))
	})
}

func segmentNamesCredential(segment string) bool {
	lower := strings.ToLower(segment)
	return otpWord.MatchString(segment) || slices.ContainsFunc(credentialWords, func(word string) bool {
		return strings.Contains(lower, word)
	})
}

func hideCredentials(v any) any {
	switch x := v.(type) {
	case []any:
		out := make([]any, len(x))
		for i, e := range x {
			out[i] = hideCredentials(e)
		}
		return out
	case map[string]any:
		out := make(map[string]any, len(x))
		for k, e := range x {
			if e != nil && namesCredential(k) {
				out[k] = marker
				continue
			}
			out[k] = hideCredentials(e)
		}
		return out
	}
	return v
}

func hideFormCredentials(form map[string][]string) map[string][]string {
	out := make(map[string][]string, len(form))
	for k, values := range form {
		if namesCredential(k) {
			values = []string{marker}
		}
		out[k] = values
	}
	return out
}
