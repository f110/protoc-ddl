package generator

import (
	"bytes"
	"testing"
)

func TestMySQLGenerator_Generate(t *testing.T) {
	buf := new(bytes.Buffer)
	tables := []*Table{
		{
			Name: "test",
			Columns: []Column{
				{Name: "id", Sequence: true},
			},
			PrimaryKey: []string{"id"},
		},
	}
	MySQLGenerator{}.Generate(buf, tables)

	t.Log(buf.String())
}
