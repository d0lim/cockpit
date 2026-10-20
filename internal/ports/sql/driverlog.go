package sqlport

import (
	"fmt"
	"log/slog"
	"strings"

	"github.com/go-sql-driver/mysql"

	"github.com/d0lim/cockpit/internal/ports"
)

type driverLog struct {
	log  *slog.Logger
	mask ports.Mask
}

func newDriverLog(log *slog.Logger, mask ports.Mask) driverLog {
	if log == nil {
		log = slog.Default()
	}
	if mask == nil {
		mask = ports.Identity
	}
	return driverLog{log: log, mask: mask}
}

func (d driverLog) Print(v ...any) {
	d.log.Warn("the mysql driver reported a connection error", "detail", d.mask(strings.TrimSpace(fmt.Sprint(v...))))
}

func RouteDriverLog(log *slog.Logger) error {
	return mysql.SetLogger(newDriverLog(log, nil))
}
