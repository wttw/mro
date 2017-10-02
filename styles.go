package main

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path"
	"strings"
	"text/template"
)

//go:generate go-bindata -prefix styles styles/...

func bootstrap(style string) {
	if style == "" {
		listStyles()
		return
	}

	files, err := AssetDir(style)
	if err != nil {
		log.Fatalf("Failed to fetch style %s: %s\n", style, err)
	}
	exists := []string{}
	for _, filename := range files {
		if filename == "description.txt" {
			continue
		}
		if strings.HasSuffix(filename, ".go.tpl") {
			filename = strings.TrimSuffix(filename, ".tpl")
		}

		_, err = os.Stat(filename)
		if err == nil {
			exists = append(exists, filename)
		}
	}
	if len(exists) > 0 {
		log.Fatalf("Exiting because files already exist: %s\n", strings.Join(exists, " "))
	}

	for _, filename := range files {
		if filename == "description.txt" {
			continue
		}

		content, err := Asset(style + "/" + filename)
		if err != nil {
			log.Fatalf("Failed to load embedded asset: %s\n", err)
		}

		if strings.HasSuffix(filename, ".mrotpl") {
			outfile := strings.TrimSuffix(filename, ".mrotpl")
			tpl, err := template.New("global").Funcs(funcs).Delims("[[", "]]").Parse(string(content))
			if err != nil {
				log.Fatalf("failed to parse template %s: %s", filename, err)
			}
			f, err := os.Create(outfile)
			if err != nil {
				log.Fatalf("failed to create %s: %s", outfile, err)
			}
			cwd, _ := os.Getwd()

			err = tpl.Execute(f, map[string]interface{}{
				"package": path.Base(cwd),
			})
			if err != nil {
				log.Fatalf("failed to execute template %s: %s", filename, err)
			}
			f.Close()
			tidyFile(outfile)
			continue
		}

		err = ioutil.WriteFile(filename, content, 0644)
		if err != nil {
			log.Fatalf("Failed to create %s: %s\n", filename, err)
		}
	}
}

func listStyles() {
	allFiles := AssetNames()
	fmt.Printf("Run \"mro --bootstrap <style>\" with one of these styles to get started\n\n")
	for _, filename := range allFiles {
		if strings.HasSuffix(filename, "/description.txt") {
			content, err := Asset(filename)
			if err != nil {
				log.Fatalf("Failed to read asset %s: %s\n", filename, err)
			}
			fmt.Println(string(content))
		}
	}
}
