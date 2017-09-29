package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"regexp"
	"strings"
	"text/template"

	"github.com/knq/snaker"
)

var funcs = template.FuncMap{
	"join":         strings.Join,
	"goname":       goname,
	"upper":        strings.ToUpper,
	"lower":        strings.ToLower,
	"title":        strings.Title,
	"camel":        snaker.SnakeToCamel,
	"snake":        snaker.CamelToSnake,
	"inc":          func(i int) int { return i + 1 },
	"names":        fieldNames,
	"excludefield": excludeField,
	"bindvars":     bindvars,
	"gonames":      gonames,
	"maybequote":   maybequote,
	"prefix":       prefix,
}

func prefix(in []string, pfx string) []string {
	s := make([]string, len(in))
	for k, v := range in {
		s[k] = pfx + v
	}
	return s
}

var badFieldNameRE = regexp.MustCompile("[^a-z_]")

func maybequote1(s string) string {
	if badFieldNameRE.MatchString(s) {
		return `"` + strings.Replace(s, `"`, `""`, -1) + `"`
	}
	return s
}

func maybequote(in interface{}) interface{} {
	f := []string{}
	switch x := in.(type) {
	case string:
		return maybequote1(x)
	case []string:
		f = x
	case []Field:
		f = fieldNames(x)
	default:
		log.Fatalf("maybequote can't take a %T\n", in)
	}

	s := make([]string, len(f))
	for k, v := range f {
		s[k] = maybequote1(v)
	}
	return s
}

func bindvars(f []Field) []string {
	s := make([]string, len(f))
	for i := 0; i < len(f); i++ {
		s[i] = fmt.Sprintf("$%d", i+1)
	}
	return s
}

func fieldNames(f []Field) []string {
	s := make([]string, len(f))
	for k, v := range f {
		s[k] = v.Name
	}
	return s
}

func gonames(f []Field, prefix string) []string {
	s := make([]string, len(f))
	for k, v := range f {
		s[k] = prefix + goname(v.Name)
	}
	return s
}

func fieldContains(f []Field, needle Field) bool {
	for _, v := range f {
		if needle.Name == v.Name {
			return true
		}
	}
	return false
}

func excludeFields(f []Field, x []Field) []Field {
	r := make([]Field, 0, len(f))
	for _, v := range f {
		if !fieldContains(x, v) {
			r = append(r, v)
		}
	}
	return r
}

func excludeField(f []Field, x Field) []Field {
	r := make([]Field, 0, len(f))
	for _, v := range f {
		if x.Name != v.Name {
			r = append(r, v)
		}
	}
	return r
}

func renderEnums(r Result) error {
	tplSource, err := ioutil.ReadFile(c.EnumTemplate)
	if err != nil {
		return err
	}
	tpl, err := template.New("enum").Funcs(funcs).Parse(string(tplSource))
	if err != nil {
		return fmt.Errorf("failed to parse enum template: %s", err)
	}
	for _, e := range r.Enums {
		err = renderEnum(e, r, tpl)
		if err != nil {
			return fmt.Errorf("while rendering %s: %s", e.Name, err)
		}
	}
	return nil
}

func renderTables(r Result) error {
	tplSource, err := ioutil.ReadFile(c.TableTemplate)
	if err != nil {
		return err
	}
	tpl, err := template.New("table").Funcs(funcs).Parse(string(tplSource))
	if err != nil {
		return fmt.Errorf("failed to parse table template: %s", err)
	}
	for _, t := range r.Tables {
		err := renderTable(t, r, tpl)
		if err != nil {
			return fmt.Errorf("while rendering %s: %s", t.Name, err)
		}
	}
	return nil
}

func tidyFile(filename string) {
	for _, pp := range c.PostProcess {
		commandline := strings.Split(pp, " ")
		commandline = append(commandline, filename)
		cmd := exec.Command(commandline[0], commandline[1:]...)
		err := cmd.Run()
		if err != nil {
			exerr, ok := err.(*exec.ExitError)
			if ok {
				fmt.Println("exiterror")
				fmt.Println(string(exerr.Stderr))
			}
			log.Fatalf("'%s' failed: %s\n", strings.Join(commandline, " "), err)

		}
	}
}

func renderEnum(e Enum, r Result, tpl *template.Template) error {
	filename := enumFilename(e)
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	err = tpl.Execute(f, struct {
		Enum   Enum
		Schema Result
		Param  map[string]interface{}
	}{
		Enum:   e,
		Schema: r,
		Param:  c.TemplateParameters,
	})

	f.Close()

	tidyFile(filename)

	return err
}

func renderTable(t Table, r Result, tpl *template.Template) error {
	filename := tableFilename(t)
	f, err := os.OpenFile(filename, os.O_APPEND|os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	err = tpl.Execute(f, struct {
		Table  Table
		Schema Result
		Param  map[string]interface{}
	}{
		Table:  t,
		Schema: r,
		Param:  c.TemplateParameters,
	})

	tidyFile(filename)

	return err
}
