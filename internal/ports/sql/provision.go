package sqlport

import (
	"crypto/rand"
	"encoding/base64"
	"fmt"
	"regexp"
	"strings"
)

const ReaderUser = "cockpit_ro"

var identifier = regexp.MustCompile(`^[A-Za-z0-9_][A-Za-z0-9_$-]{0,62}$`)

type statement struct {
	text string
	args []any
}

func checkIdentifiers(kind string, names ...string) error {
	for _, n := range names {
		if !identifier.MatchString(n) {
			return fmt.Errorf("%s %q is not a plain identifier", kind, n)
		}
	}
	return nil
}

func newPassword() (string, error) {
	buf := make([]byte, 24)
	if _, err := rand.Read(buf); err != nil {
		return "", err
	}
	return base64.RawURLEncoding.EncodeToString(buf), nil
}

func mysqlIdent(name string) string {
	return "`" + strings.ReplaceAll(name, "`", "``") + "`"
}

func pgIdent(name string) string {
	return `"` + strings.ReplaceAll(name, `"`, `""`) + `"`
}

func pgLiteral(value string) string {
	return "'" + strings.ReplaceAll(value, "'", "''") + "'"
}

func mysqlProvision(schemas []string, password string) []statement {
	out := []statement{
		{"CREATE USER IF NOT EXISTS ?@'%' IDENTIFIED BY ?", []any{ReaderUser, password}},
		{"ALTER USER ?@'%' IDENTIFIED BY ?", []any{ReaderUser, password}},
	}
	for _, s := range schemas {
		out = append(out, statement{"GRANT SELECT ON " + mysqlIdent(s) + ".* TO ?@'%'", []any{ReaderUser}})
	}
	return out
}

func pgProvision(database string, schemas []string, owner, password string) []statement {
	reader := pgIdent(ReaderUser)
	out := []statement{
		{"DO $cockpit$ BEGIN IF NOT EXISTS (SELECT FROM pg_catalog.pg_roles WHERE rolname = " + pgLiteral(ReaderUser) + ") THEN CREATE ROLE " + reader + " LOGIN; END IF; END $cockpit$", nil},
		{"ALTER ROLE " + reader + " WITH LOGIN NOSUPERUSER NOCREATEDB NOCREATEROLE NOINHERIT PASSWORD " + pgLiteral(password), nil},
		{"GRANT CONNECT ON DATABASE " + pgIdent(database) + " TO " + reader, nil},
	}
	for _, s := range schemas {
		out = append(out,
			statement{"GRANT USAGE ON SCHEMA " + pgIdent(s) + " TO " + reader, nil},
			statement{"GRANT SELECT ON ALL TABLES IN SCHEMA " + pgIdent(s) + " TO " + reader, nil},
		)
		if owner != "" {
			out = append(out, statement{"ALTER DEFAULT PRIVILEGES FOR ROLE " + pgIdent(owner) + " IN SCHEMA " + pgIdent(s) + " GRANT SELECT ON TABLES TO " + reader, nil})
		}
	}
	return out
}
