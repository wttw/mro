package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"html/template"
	"io/ioutil"
	"log"
	"os"
	"path"

	"github.com/jackc/pgx"

	"github.com/hashicorp/hcl"
)

var configFile string
var initFiles bool
var clean bool
var defaultPackage string
var c Config
var db *pgx.Conn

func main() {
	flag.Parse()

	if initFiles {
		bootstrap(flag.Arg(0))
		return
	}

	cfg, err := ioutil.ReadFile(configFile)
	if err != nil {
		log.Fatalf("Cannot read configuration file '%s': %s", configFile, err)
	}

	err = hcl.Unmarshal(cfg, &c)
	if err != nil {
		log.Fatalf("Failed to read configuration '%s': %s", configFile, err)
	}

	_, ok := c.TemplateParameters["package"]
	if !ok {
		c.TemplateParameters["package"] = defaultPackage
	}

	dbCfg, err := pgx.ParseConnectionString(c.ConnectionString)
	if err != nil {
		log.Fatalf("Invalid connection string in '%s': %s", configFile, err)
	}

	db, err = pgx.Connect(dbCfg)
	if err != nil {
		log.Fatalf("Failed to connect to database: %s", err)
	}

	schema := introspect()
	if c.JsonOutput != "" {
		b, err := json.MarshalIndent(&schema, "", "  ")
		if err != nil {
			log.Fatalf("%s", err)
		}
		err = ioutil.WriteFile(c.JsonOutput, b, 0644)
		if err != nil {
			log.Fatalf("%s", err)
		}
	}

	if clean {
		wipeFiles(schema)
		return
	}

	wipeFiles(schema)

	if c.EnumFilename != "" {
		err = renderEnums(schema)
		if err != nil {
			log.Fatalf("Failed to render enums: %s", err)
		}
	}
	if c.TableFilename != "" {
		err = renderTables(schema)
		if err != nil {
			log.Fatalf("Failed to render tables: %s", err)
		}
	}
}

func init() {
	cwd, _ := os.Getwd()

	flag.StringVar(&configFile, "config", "mro.cfg", "Read configuration from this file")
	flag.BoolVar(&clean, "clean", false, "Delete generated files")
	flag.BoolVar(&initFiles, "bootstrap", false, "Initialize configuration files")
	flag.StringVar(&defaultPackage, "package", path.Base(cwd), "Generate files for this package")
}

func tableFilename(t Table) string {
	if c.TableFilename == "" {
		return ""
	}
	filenameTemplate, err := template.New("filename").Parse(c.TableFilename)
	if err != nil {
		log.Fatalf("Bad template for TableFilename: %s\n", err)
	}
	var b bytes.Buffer
	err = filenameTemplate.Execute(&b, t)
	if err != nil {
		log.Fatalf("TableFilename template failed: %s\n", err)
	}
	return b.String()
}

func enumFilename(e Enum) string {
	if c.EnumFilename == "" {
		return ""
	}
	filenameTemplate, err := template.New("filename").Parse(c.EnumFilename)
	if err != nil {
		log.Fatalf("Bad template for EnumFilename: %s\n", err)
	}
	var b bytes.Buffer
	err = filenameTemplate.Execute(&b, e)
	if err != nil {
		log.Fatalf("EnumFilename template failed: %s\n", err)
	}
	return b.String()
}

func wipeFiles(r Result) {
	for _, t := range r.Tables {
		filename := tableFilename(t)
		if filename != "" {
			_ = os.Remove(filename)
		}
	}
	for _, e := range r.Enums {
		filename := enumFilename(e)
		if filename != "" {
			_ = os.Remove(filename)
		}
	}
}
