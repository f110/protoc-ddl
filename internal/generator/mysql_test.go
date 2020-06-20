package generator

import (
	"bytes"
	"testing"

	"go.f110.dev/protoc-ddl/internal/schema"
)

func TestMySQLGenerator_Generate(t *testing.T) {
	buf := new(bytes.Buffer)
	tables := []*schema.Table{
		{
			Name: "test",
			Columns: []schema.Column{
				{Name: "id", Sequence: true},
			},
			PrimaryKey: []string{"id"},
		},
	}
	MySQLGenerator{}.Generate(buf, tables)

	t.Log(buf.String())
}
