package generator

import (
	"bytes"
	"testing"

	"github.com/pingcap/parser"
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
			Query:    "select * FROM `user` WHERE id = ?",
			Rendered: "SELECT * FROM `user` WHERE `id` = ?",
		},
		{
			Query:    "select * from user where age > 20",
			Rendered: "SELECT * FROM `user` WHERE `age` > 20",
		},
		{
			Query:    "SELECT `id`, `name` FROM `user` WHERE `id` = ?",
			Rendered: "SELECT `id`, `name` FROM `user` WHERE `id` = ?",
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
	}

	const debug = false

	p := parser.New()
	for _, c := range cases {
		tt := c
		t.Run("", func(t *testing.T) {
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
