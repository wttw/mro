package {{.Param.package}}
{{ $stable := printf "%s.%s" (maybequote .Table.Schema) (maybequote .Table.Name) }}
// {{ $stable }}

import (
    "errors"
    "time"
    "net"
    "github.com/jackc/pgx"
    "github.com/jackc/pgx/pgtype"
    "database/sql"
    "github.com/lib/pq"
)

//  {{$goname := goname .Table.Name}}{{$goname}} represents a row from {{.Table.Name}}
type {{$goname}} struct { {{- range $f := .Table.Fields}}
  {{goname $f.Name}} {{$f.GoType}} `json:"{{$f.Name}}"`{{end}}
}

const {{$goname}}Columns = `{{join (maybequote .Table.Fields) ", "}}`

{{if .Table.IDField.Name}}
// Insert a {{$goname}} into the database
func (t *{{$goname}}) Insert(db MRODB) error {
    {{- $dfields := excludefield .Table.Fields .Table.IDField}}
    const sql = `insert into {{ $stable }} (` +
      `{{join (maybequote $dfields) ", "}}` +
      `) values (` +
      `{{join (bindvars $dfields) ", "}}` +
      `) returning {{maybequote .Table.IDField.Name}}`
    err := db.QueryRow(sql, {{join (gonames $dfields "t.") ", "}}).Scan(&t.{{goname .Table.IDField.Name}})
    if err != nil {
        return err
    }
    return nil
}

// Update an existing {{$goname}} in the database
func (t *{{$goname}}) Update(db MRODB) error {
    {{- $dfields := excludefield .Table.Fields .Table.IDField}}
    const sql = `update {{$stable}} set (` +
      `{{join (maybequote $dfields) ", "}}` +
      `) = (` +
      `{{join (bindvars $dfields) ", "}}` +
      `) where {{maybequote .Table.IDField.Name}} = ${{inc (len $dfields)}}`

    _, err := db.Exec(sql, {{join (gonames $dfields "t.") ", "}}, t.{{goname .Table.IDField.Name}})
    return err
}

// Upsert a {{$goname}} into the database
func (t *{{$goname}}) Upsert(db MRODB) error {
    const sql = `insert into {{ $stable }} (` +
      `{{join (maybequote .Table.Fields) ", "}}` +
      `) values (` +
      `{{join (bindvars .Table.Fields) ", "}}` +
      `) on conflict ({{maybequote .Table.IDField.Name}}) do update set (` +
      `{{join (maybequote .Table.Fields) ", "}}` +
      `) = (` +
      `{{join (prefix (maybequote .Table.Fields) "EXCLUDED.") ", "}}` +
      `)`

    _, err := db.Exec(sql, {{join (gonames .Table.Fields "t.") ", "}})
    return err
}

// Delete a {{$goname}} from the database
func (t *{{$goname}}) Delete(db MRODB) error {
    const sql = `delete from {{ $stable }} where {{maybequote .Table.IDField.Name}} = $1`

    _, err := db.Exec(sql, t.{{goname .Table.IDField.Name}})
    return err
}
{{else}}
// Insert a {{$goname}} into the database
func (t *{{$goname}}) Insert(db MRODB) error {
    const sql = `insert into {{ $stable }} (` +
      `{{join (maybequote .Table.Fields) ", "}}` +
      `) values (` +
      `{{join (bindvars .Table.Fields) ", "}}` +
      `)`
    _, err := db.Query(sql, {{join (gonames .Table.Fields "t.") ", "}})
    if err != nil {
        return err
    }
    return nil
}
{{end}}

func FetchAll{{$goname}}(db MRODB) ([]{{$goname}}, error) {
    const sql = `select ` +
      `{{join (maybequote .Table.Fields) ", "}}` +
      `from {{$stable}}`

    q, err := db.Query(sql)
    if err != nil {
        return nil, err
    }
    defer q.Close()
    result := []{{$goname -}} {}
    for q.Next() {
        var row {{$goname}}
        err = q.Scan({{join (gonames .Table.Fields "&row.") ", "}})
        if err != nil {
            return nil, err
        }
        result = append(result, row)
    }
    return result, nil
}

func UnmarshalOne{{$goname}}(row *pgx.Row, r *{{$goname}}) error {
    return row.Scan({{join (gonames .Table.Fields "&r.") ", "}})
}

func Unmarshal{{$goname}}(q *pgx.Rows) ([]{{$goname}}, error) {
    result := []{{$goname}}{}
    for q.Next() {
        row := {{$goname}}{}
        err := q.Scan({{join (gonames .Table.Fields "&row.") ", "}})
        if err != nil {
            return nil, err
        }
        result = append(result, row)
    }
    return result, nil
}

{{ $t := .Table }}
{{range $q := .Table.Queries}}
{{if $q.SingleRow}}
// {{$q.Name}} returns the result of
//   {{$q.Query}}
{{if ne $q.Query $q.OriginalQuery}}//   (originally {{$q.OriginalQuery}}){{end}}
func {{$q.Name}}(db MRODB{{range $p := $q.Parameters -}}
, {{$p.Name}} {{$p.GoType}}
{{- end}}) ({{$goname}}, error) {
  const sql = `{{$q.Query}}`
  var row {{$goname}}
  err := db.QueryRow(sql, {{join (names $q.Parameters) ", "}}).Scan({{join (gonames $t.Fields "&row.") ", "}})
  return row, nil
}
{{else}}
// {{$q.Name}} returns the result of
//   {{$q.Query}}
{{if ne $q.Query $q.OriginalQuery}}//   (originally {{$q.OriginalQuery}}){{end}}
func {{$q.Name}}(db MRODB{{range $p := $q.Parameters -}}
, {{$p.Name}} {{$p.GoType}}
{{- end}}) ([]{{$goname}}, error) {
  result := []{{$goname}}{}
  const sql = `{{$q.Query}}`
  q, err := db.Query(sql, {{join (names $q.Parameters) ", "}})
  if err != nil {
      return nil, err
  }
  defer q.Close()
  for q.Next() {
      row := {{$goname}}{}
      err = q.Scan({{join (gonames $t.Fields "&row.") ", "}})
      if err != nil {
          return nil, err
      }
      result = append(result, row)
  }
  return result, nil
}
{{end}}
{{end}}