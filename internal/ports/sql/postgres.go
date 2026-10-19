package sqlport

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/stdlib"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
)

var pgDenied = map[string]bool{"42501": true, "25006": true, "28P01": true, "28000": true}

type pgBackend struct {
	decl   contract.Database
	target Target
	max    limits
}

func clearPGEnv() {
	for _, kv := range os.Environ() {
		if name, _, _ := strings.Cut(kv, "="); strings.HasPrefix(name, "PG") {
			_ = os.Unsetenv(name)
		}
	}
}

func (b pgBackend) config(user, password string, params map[string]string) (*pgx.ConnConfig, error) {
	cfg, err := pgx.ParseConfig("sslmode=disable")
	if err != nil {
		return nil, err
	}
	cfg.Host = b.target.Host
	cfg.Port = uint16(b.target.Port)
	cfg.Database = b.target.Database
	cfg.User = user
	cfg.Password = password
	cfg.TLSConfig = nil
	cfg.Fallbacks = nil
	cfg.ConnectTimeout = dialTimeout
	cfg.KerberosSrvName = ""
	cfg.KerberosSpn = ""
	cfg.ValidateConnect = nil
	cfg.RuntimeParams = params
	return cfg, nil
}

func (b pgBackend) readerParams() map[string]string {
	return map[string]string{
		"default_transaction_read_only": "on",
		"statement_timeout":             strconv.FormatInt(b.max.timeout.Milliseconds(), 10),
		"application_name":              "cockpit",
	}
}

func (b pgBackend) connect(user, password string, params map[string]string) *sql.DB {
	cfg, err := b.config(user, password, params)
	if err != nil {
		return sql.OpenDB(failingConnector{err})
	}
	return stdlib.OpenDB(*cfg)
}

func (b pgBackend) provision(ctx context.Context, rootPassword, password string) error {
	db := b.connect(b.target.RootUser, rootPassword, map[string]string{"application_name": "cockpit"})
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, s := range pgProvision(b.target.Database, b.decl.ReadSchemas, b.decl.OwnerRole, password) {
		if _, err := db.ExecContext(ctx, s.text); err != nil {
			return err
		}
	}
	return nil
}

func (b pgBackend) open(password string) *sql.DB {
	return b.connect(ReaderUser, password, b.readerParams())
}

func (b pgBackend) classify(err error) (engine.PortCode, string) {
	var pe *pgconn.PgError
	if !errors.As(err, &pe) {
		if argumentMismatch(err) {
			return engine.PortBadArgs, "the params do not match the placeholders of the statement"
		}
		if pgconn.Timeout(err) {
			return engine.PortTimeout, "the query exceeded its time limit"
		}
		return engine.PortUnavailable, "failed to reach the database"
	}
	reason := fmt.Sprintf("postgres %s: %s", pe.Code, pe.Message)
	switch {
	case pgDenied[pe.Code]:
		return engine.PortDenied, "permission denied (" + reason + ")"
	case pe.Code == "08P01" && argumentMismatch(pe):
		return engine.PortBadArgs, "the params do not match the placeholders of the statement (" + reason + ")"
	case pe.Code == "57014":
		return engine.PortTimeout, "the server time limit stopped the query (" + reason + ")"
	case strings.HasPrefix(pe.Code, "42"):
		return engine.PortBadArgs, "the statement was rejected (" + reason + ")"
	}
	return engine.PortUnavailable, "the query failed (" + reason + ")"
}

func (b pgBackend) authFailure(err error) bool {
	var pe *pgconn.PgError
	return errors.As(err, &pe) && (pe.Code == "28P01" || pe.Code == "28000")
}
