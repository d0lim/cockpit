package sqlport

import (
	"context"
	"database/sql"
	"database/sql/driver"
	"errors"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/go-sql-driver/mysql"

	"github.com/d0lim/cockpit/internal/contract"
	"github.com/d0lim/cockpit/internal/engine"
)

const dialTimeout = 5 * time.Second

var mysqlDenied = map[uint16]bool{1044: true, 1045: true, 1142: true, 1143: true, 1227: true, 1370: true, 1792: true}

type mysqlBackend struct {
	decl   contract.Database
	target Target
	max    limits
}

func (b mysqlBackend) config(user, password string) *mysql.Config {
	cfg := mysql.NewConfig()
	cfg.User = user
	cfg.Passwd = password
	cfg.Net = "tcp"
	cfg.Addr = net.JoinHostPort(b.target.Host, strconv.Itoa(b.target.Port))
	cfg.Timeout = dialTimeout
	cfg.ParseTime = true
	cfg.Loc = time.UTC
	cfg.AllowAllFiles = false
	cfg.MultiStatements = false
	return cfg
}

func (b mysqlBackend) readerConfig(password string) *mysql.Config {
	cfg := b.config(ReaderUser, password)
	cfg.Params = map[string]string{
		"transaction_read_only": "1",
		"max_execution_time":    strconv.FormatInt(b.max.timeout.Milliseconds(), 10),
		"sql_select_limit":      strconv.Itoa(b.max.rows + 1),
	}
	return cfg
}

func (b mysqlBackend) connect(cfg *mysql.Config) *sql.DB {
	conn, err := mysql.NewConnector(cfg)
	if err != nil {
		return sql.OpenDB(failingConnector{err})
	}
	return sql.OpenDB(conn)
}

func (b mysqlBackend) provision(ctx context.Context, rootPassword, password string) error {
	cfg := b.config(b.target.RootUser, rootPassword)
	cfg.InterpolateParams = true
	db := b.connect(cfg)
	defer db.Close()
	db.SetMaxOpenConns(1)
	for _, s := range mysqlProvision(b.decl.ReadSchemas, password) {
		if _, err := db.ExecContext(ctx, s.text, s.args...); err != nil {
			return err
		}
	}
	return nil
}

func (b mysqlBackend) open(password string) *sql.DB {
	return b.connect(b.readerConfig(password))
}

func (b mysqlBackend) classify(err error) (engine.PortCode, string) {
	var me *mysql.MySQLError
	if !errors.As(err, &me) {
		return engine.PortUnavailable, "failed to reach the database"
	}
	reason := fmt.Sprintf("mysql %d: %s", me.Number, me.Message)
	switch {
	case mysqlDenied[me.Number]:
		return engine.PortDenied, "permission denied (" + reason + ")"
	case me.Number == 3024:
		return engine.PortTimeout, "the server time limit stopped the query (" + reason + ")"
	case me.Number == 1064 || me.Number == 1146 || me.Number == 1054 || me.Number == 1049:
		return engine.PortBadArgs, "the statement was rejected (" + reason + ")"
	}
	return engine.PortUnavailable, "the query failed (" + reason + ")"
}

func (b mysqlBackend) authFailure(err error) bool {
	var me *mysql.MySQLError
	return errors.As(err, &me) && me.Number == 1045
}

type failingConnector struct{ err error }

func (c failingConnector) Connect(context.Context) (driver.Conn, error) { return nil, c.err }

func (c failingConnector) Driver() driver.Driver { return failingDriver(c) }

type failingDriver struct{ err error }

func (d failingDriver) Open(string) (driver.Conn, error) { return nil, d.err }
