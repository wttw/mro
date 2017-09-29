package {{.Param.package}}

import (
    "encoding/json"
    "errors"
    "database/sql/driver"
)

//  {{$goname := goname .Enum.Name}}{{$goname}} represents the {{.Enum.Name}} enum
type {{$goname}} uint16

const ({{range $i, $label := .Enum.Labels}}
  // {{$label}}
  {{$goname}}{{goname $label}}{{if eq $i 0}} = iota{{end}}
{{- end}}
)

// String returns the string value of the label
func (e {{$goname}}) String() string {
  switch e { {{- range $label := .Enum.Labels}}
    case {{$goname}}{{goname $label}}:
      return "{{$label}}"
{{end}}
  }
  return ""
}

// MarshalText marshals {{$goname}} into text
func (e {{$goname}}) MarshalText() ([]byte, error) {
  return []byte(e.String()), nil
}

// UnmarshalText unmarshals {{$goname}} from text
func (e *{{$goname}}) UnmarshalText(text []byte) error {
    switch string(text) { {{- range $label := .Enum.Labels}}
        case "{{$label}}":
            *e = {{$goname}}{{goname $label}}
{{end}}
        default:
            return errors.New("invalid {{$goname}}")
    }
    return nil
}

// Value satisfies sql/driver.Valuer
func (e {{$goname}}) Value() (driver.Value, error) {
    return e.String(), nil
}

// Scan satisfies sql.Scanner
func (e *{{$goname}}) Scan(src interface{}) error {
    buf, ok := src.([]byte)
    if !ok {
        return errors.New("invalid {{$goname}}")
    }
    return e.UnmarshalText(buf)
}

// Valid{{$goname}} provides all the valid enum labels
func Valid{{$goname}}() []string {
    return []string{ {{- range $i, $label := .Enum.Labels}}{{if ne $i 0}}, {{end}}"{{$label}}"{{end -}} }
}

// MarshalJSON for making JSON
func (e {{$goname}}) MarshalJSON() ([]byte, error) {
    return json.Marshal(e.String())
}

// UnmarshalJSON for hydrating from json
func (e *{{$goname}}) UhnmarshalJSON(data []byte) error {
    return e.UnmarshalText(data)
}
