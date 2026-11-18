package main

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"net/http"
	"strings"
)

const bigBodySize = 3 << 19

func (s *wireServer) shapes(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"numeric": map[string]any{"password": 12345},
		"public":  map[string]string{"token": "tok-0a1b2c3d4e5f"},
		"private": map[string]string{"token": derive(s.appToken, "shape")},
	})
}

func segment(v any) string {
	data, _ := json.Marshal(v)
	return base64.RawURLEncoding.EncodeToString(data)
}

func (s *wireServer) token(w http.ResponseWriter, r *http.Request) {
	head := segment(map[string]string{"alg": "HS256", "typ": "JWT"})
	claims := segment(map[string]string{"sub": "user-7", "aud": "ledger", "note": s.dbPassword, "apiToken": derive(s.appToken, "claim")})
	mac := hmac.New(sha256.New, []byte(s.appToken))
	mac.Write([]byte(head + "." + claims))
	w.Header().Set("Content-Type", "application/jwt")
	w.Write([]byte(head + "." + claims + "." + base64.RawURLEncoding.EncodeToString(mac.Sum(nil))))
}

func pemPart(text string) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "text/plain")
		w.Write([]byte(text))
	}
}

func big(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"fill": strings.Repeat("x", bigBodySize)})
}

func binary(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/octet-stream")
	w.Write([]byte{0x89, 'P', 'N', 'G', 0, 1, 2, 3, 0xff, 0xfe, 0, 4})
}
