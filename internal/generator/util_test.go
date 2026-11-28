package generator

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.f110.dev/protoc-ddl/internal/schema"
)

func TestAstFormatter(t *testing.T) {
	cases := []struct {
		Query    string
		Rendered string
	}{
		{
			Query:    "SELECT * FROM `user`",
			Rendered: "SELECT * FROM `user`",
		},
		{
			Query:    "SELECT 1 + 1",
			Rendered: "SELECT 1 + 1",
		},
		{
			Query:    "SELECT VERSION()",
			Rendered: "SELECT VERSION()",
		},
		{
			Query:    "SELECT * FROM `:table_name:`",
			Rendered: "SELECT * FROM `user`",
		},
		{
			Query:    "SELECT DISTINCT `name` FROM `user`",
			Rendered: "SELECT DISTINCT `name` FROM `user`",
		},
		{
			Query:    "SELECT count(distinct `age`) FROM `user`",
			Rendered: "SELECT COUNT(distinct `age`) FROM `user`",
		},
	}

	const debug = false

	p := parser.New()
	for i, c := range cases {
		tt := c
		t.Run(fmt.Sprintf("%d", i), func(t *testing.T) {
			s, w, err := p.Parse(tt.Query, "", "")
			require.NoError(t, err)
			require.Len(t, w, 0)

			formatter := NewQueryFormatter(&schema.Message{TableName: "user"}, s[0])
			formatter.debug = debug

			buf := new(bytes.Buffer)
			formatter.Format(buf)
			assert.Equal(t, tt.Rendered, buf.String())
		})
	}
}
