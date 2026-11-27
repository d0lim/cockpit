package contract

import (
	"cmp"
	"fmt"
	"regexp"
)

const (
	DefaultDeclaration     = ".cockpit/project.yaml"
	DeclarationFilePattern = `^\.cockpit/[a-z0-9][a-z0-9-]{0,62}\.yaml$`
)

var declarationFile = regexp.MustCompile(DeclarationFilePattern)

type DeclarationFile string

func ParseDeclarationFile(raw string) (DeclarationFile, error) {
	if raw == "" || raw == DefaultDeclaration {
		return "", nil
	}
	if !declarationFile.MatchString(raw) {
		return "", fmt.Errorf("the declaration must be a file .cockpit/<name>.yaml whose name is lowercase letters, digits and -, got %q", raw)
	}
	return DeclarationFile(raw), nil
}

func (f DeclarationFile) Path() string { return cmp.Or(string(f), DefaultDeclaration) }

func (f *DeclarationFile) UnmarshalText(text []byte) error {
	parsed, err := ParseDeclarationFile(string(text))
	if err != nil {
		return err
	}
	*f = parsed
	return nil
}
