package totp

import (
	"encoding/json"
	"errors"
	"os"
)

const section = "local"

type SeedFile struct {
	Path string
}

func ReadPrivateEnv(path string) (map[string]string, error) {
	_, local, err := readDoc(path)
	return local, err
}

func (f SeedFile) Save(key, secret string) (map[string]string, error) {
	doc, local, err := readDoc(f.Path)
	if err != nil {
		return nil, err
	}
	local[key] = secret
	doc[section] = local
	data, err := json.MarshalIndent(doc, "", "  ")
	if err != nil {
		return nil, err
	}
	fd, err := os.OpenFile(f.Path, os.O_WRONLY|os.O_TRUNC, 0)
	if err != nil {
		return nil, err
	}
	if _, err := fd.Write(append(data, '\n')); err != nil {
		fd.Close()
		return nil, err
	}
	if err := fd.Close(); err != nil {
		return nil, err
	}
	return local, nil
}

func readDoc(path string) (map[string]any, map[string]string, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, nil, err
	}
	var doc map[string]any
	if err := json.Unmarshal(raw, &doc); err != nil {
		return nil, nil, errors.New("private env 가 JSON 객체가 아니다")
	}
	values, ok := doc[section].(map[string]any)
	if !ok {
		return nil, nil, errors.New("private env 에 \"" + section + "\" 절이 없다")
	}
	local := map[string]string{}
	for k, v := range values {
		s, ok := v.(string)
		if !ok {
			return nil, nil, errors.New("private env 의 " + k + " 값이 문자열이 아니다")
		}
		local[k] = s
	}
	return doc, local, nil
}
