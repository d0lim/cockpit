package sqlport

import (
	"context"
	"database/sql"
	"fmt"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/d0lim/cockpit/internal/ports"
)

var binaryTypes = map[string]bool{
	"BLOB": true, "TINYBLOB": true, "MEDIUMBLOB": true, "LONGBLOB": true,
	"BINARY": true, "VARBINARY": true, "BYTEA": true, "BIT": true, "GEOMETRY": true,
}

func run(ctx context.Context, pool *sql.DB, stmt string, params []any, l limits, mask ports.Mask) (Result, error) {
	tx, err := pool.BeginTx(ctx, &sql.TxOptions{ReadOnly: true})
	if err != nil {
		return Result{}, err
	}
	defer func() { _ = tx.Rollback() }()
	rows, err := tx.QueryContext(ctx, stmt, params...)
	if err != nil {
		return Result{}, err
	}
	defer rows.Close()
	return read(rows, l, mask)
}

func read(rows *sql.Rows, l limits, mask ports.Mask) (Result, error) {
	columns, err := rows.Columns()
	if err != nil {
		return Result{}, err
	}
	types, err := rows.ColumnTypes()
	if err != nil {
		return Result{}, err
	}
	names := make([]string, len(columns))
	for i, c := range columns {
		names[i] = mask(c)
	}
	res := Result{Columns: names, Rows: [][]any{}}
	size := 0
	for rows.Next() {
		if len(res.Rows) == l.rows {
			res.Truncated = true
			break
		}
		cells := make([]any, len(columns))
		ptrs := make([]any, len(columns))
		for i := range cells {
			ptrs[i] = &cells[i]
		}
		if err := rows.Scan(ptrs...); err != nil {
			return Result{}, err
		}
		row, n := convert(cells, types, mask)
		if size+n > l.bytes {
			res.Truncated = true
			break
		}
		size += n
		res.Rows = append(res.Rows, row)
	}
	if err := rows.Err(); err != nil {
		return Result{}, err
	}
	return res, nil
}

func convert(cells []any, types []*sql.ColumnType, mask ports.Mask) ([]any, int) {
	out := make([]any, len(cells))
	size := 0
	for i, v := range cells {
		var cell any
		switch x := v.(type) {
		case nil:
			size += 4
		case []byte:
			if binaryTypes[strings.ToUpper(types[i].DatabaseTypeName())] || !utf8.Valid(x) {
				cell = fmt.Sprintf("<%d bytes>", len(x))
			} else {
				cell = mask(string(x))
			}
		case string:
			cell = mask(x)
		case time.Time:
			cell = x.UTC().Format(time.RFC3339Nano)
		case int64, int32, int16, int8, int, uint64, uint32, uint16, uint8, uint, float64, float32, bool:
			cell = x
		default:
			cell = mask(fmt.Sprint(x))
		}
		if s, ok := cell.(string); ok {
			size += len(s) + 2
		} else if cell != nil {
			size += 8
		}
		out[i] = cell
	}
	return out, size
}
