package {{.Param.package}}

import (
    "github.com/jackc/pgx"
)

type MRODB interface {
    Exec(string, ...interface{}) (pgx.CommandTag, error)
    Query(string, ...interface{}) (*pgx.Rows, error)
    QueryRow(string, ...interface{}) *pgx.Row
}