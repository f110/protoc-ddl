package main

import (
	"context"
	"fmt"
	"os"

	"github.com/spf13/pflag"
	"go.f110.dev/xerrors"

	"go.f110.dev/protoc-ddl/internal/migrate"
)

type options struct {
	SchemaFile string
	Driver     string
	DSN        string
	Execute    bool
}

func do(args []string) error {
	opt := &options{}
	fs := pflag.NewFlagSet("migrate", pflag.ContinueOnError)
	fs.StringVar(&opt.SchemaFile, "schema", "", "Schema file path")
	fs.StringVar(&opt.Driver, "driver", "", "Database driver name")
	fs.StringVar(&opt.DSN, "dsn", "", "DSN of target database")
	fs.BoolVarP(&opt.Execute, "execute", "f", false, "Execute migration")
	if err := fs.Parse(args); err != nil {
		return xerrors.WithStack(err)
	}

	m, err := migrate.NewMigration(opt.SchemaFile, opt.Driver, opt.DSN)
	if err != nil {
		return err
	}

	err = m.Execute(context.Background(), opt.Execute)
	if err != nil {
		return err
	}

	return nil
}

func main() {
	if err := do(os.Args[1:]); err != nil {
		fmt.Fprintf(os.Stderr, "%+v", err)
		os.Exit(1)
	}
}
