package main

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"go/format"
	"io/ioutil"
	"os"

	"github.com/spf13/pflag"
)

func main() {
	packageName := "main"
	lang := "go"
	outfile := ""
	fs := pflag.NewFlagSet("schema_hash", pflag.PanicOnError)
	fs.StringVar(&packageName, "package", packageName, "The name of the package")
	fs.StringVar(&lang, "lang", lang, "Language")
	fs.StringVar(&outfile, "outfile", outfile, "Output file. If empty, will output to stdout")
	fs.Parse(os.Args[1:])

	if len(fs.Args()) != 1 {
		fmt.Fprintln(os.Stderr, "Usage: schema_hash [filename]")
		os.Exit(1)
	}

	schemaFile := fs.Args()[0]
	if _, err := os.Stat(schemaFile); os.IsNotExist(err) {
		fmt.Fprintf(os.Stderr, "%s not found\n", schemaFile)
		os.Exit(1)
	}

	b, err := ioutil.ReadFile(schemaFile)
	if err != nil {
		fmt.Fprintf(os.Stderr, "Could not read schema file: %v\n", err)
		os.Exit(1)
	}

	h := sha256.Sum256(b)
	schemaHash := hex.EncodeToString(h[:])

	var formatted []byte
	buf := new(bytes.Buffer)
	if lang == "go" {
		fmt.Fprintf(buf, "package %s\n", packageName)
		fmt.Fprintln(buf, "const SchemaHash = \""+schemaHash+"\"")
		f, err := format.Source(buf.Bytes())
		if err != nil {
			fmt.Fprint(os.Stderr, buf.Bytes())
			fmt.Fprintf(os.Stderr, "Failed format: %v\n", err)
			os.Exit(1)
		}
		formatted = f
	}
	if outfile != "" {
		ioutil.WriteFile(outfile, formatted, 0644)
	} else {
		os.Stdout.Write(formatted)
	}
}
