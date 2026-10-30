package generator

import (
	"testing"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.f110.dev/protoc-ddl/internal/schema"
)

func TestNewWhereNode(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	m := &schema.Message{
		TableName:   "item",
		PrimaryKeys: []*schema.Field{pk},
		Fields: schema.NewFields([]*schema.Field{
			pk,
			{Name: "status", Type: "TYPE_UINT32"},
			{Name: "title", Type: "TYPE_STRING"},
			{Name: "body", Type: "TYPE_STRING"},
			{Name: "deleted_at", Type: schema.TimestampType, Null: true},
		}),
	}
	p := parser.New()
	parse := func(t *testing.T, where string) ast.ExprNode {
		stmt, _, err := p.Parse("SELECT * FROM `item` WHERE "+where, "", "")
		require.NoError(t, err)
		return stmt[0].(*ast.SelectStmt).Where
	}

	cases := []struct {
		where    string
		wantExpr string
		wantArgs []string
	}{
		{
			where:    "`status` IN (?)",
			wantExpr: "whereIn(\"`status`\", status)",
			wantArgs: []string{"status []uint32"},
		},
		{
			where:    "`title` LIKE ?",
			wantExpr: "whereCond(\"`title` LIKE ?\", title)",
			wantArgs: []string{"title string"},
		},
		{
			where:    "`title` NOT LIKE ?",
			wantExpr: "whereCond(\"`title` NOT LIKE ?\", title)",
			wantArgs: []string{"title string"},
		},
		{
			where:    "(`id` >= ?)",
			wantExpr: "whereCond(\"`id` >= ?\", id)",
			wantArgs: []string{"id int32"},
		},
		{
			where:    "`id` = ? AND `status` = 1",
			wantExpr: "whereAnd(whereCond(\"`id` = ?\", id), whereCond(\"`status` = 1\"))",
			wantArgs: []string{"id int32"},
		},
		{
			where:    "`title` = 'it''s' AND `body` != ?",
			wantExpr: "whereAnd(whereCond(\"`title` = ?\", \"it's\"), whereCond(\"`body` != ?\", body))",
			wantArgs: []string{"body string"},
		},
		{
			where:    "`deleted_at` IS NULL OR `deleted_at` IS NOT NULL",
			wantExpr: "whereOr(whereCond(\"`deleted_at` IS NULL\"), whereCond(\"`deleted_at` IS NOT NULL\"))",
		},
		{
			where:    "`id` = ? AND `status` = ? AND `title` = ?",
			wantExpr: "whereAnd(whereCond(\"`id` = ?\", id), whereCond(\"`status` = ?\", status), whereCond(\"`title` = ?\", title))",
			wantArgs: []string{"id int32", "status uint32", "title string"},
		},
		{
			where:    "(`id` = ? AND `status` = ?) AND `title` = ?",
			wantExpr: "whereAnd(whereCond(\"`id` = ?\", id), whereCond(\"`status` = ?\", status), whereCond(\"`title` = ?\", title))",
			wantArgs: []string{"id int32", "status uint32", "title string"},
		},
		{
			where:    "`status` IN (?) AND (`title` LIKE ? OR `body` LIKE ?)",
			wantExpr: "whereAnd(whereIn(\"`status`\", status), whereOr(whereCond(\"`title` LIKE ?\", title), whereCond(\"`body` LIKE ?\", body)))",
			wantArgs: []string{"status []uint32", "title string", "body string"},
		},
		{
			where:    "`title` LIKE ? OR `title` = ? OR `title` IN (?)",
			wantExpr: "whereOr(whereCond(\"`title` LIKE ?\", title), whereCond(\"`title` = ?\", title2), whereIn(\"`title`\", title3))",
			wantArgs: []string{"title string", "title2 string", "title3 []string"},
		},
	}
	for _, tc := range cases {
		t.Run(tc.where, func(t *testing.T) {
			n, err := newWhereNode(m, parse(t, tc.where))
			require.NoError(t, err)
			assert.Equal(t, tc.wantExpr, n.goExpr())
			var args []string
			for _, a := range n.args() {
				args = append(args, a.name+" "+a.goType())
			}
			assert.Equal(t, tc.wantArgs, args)
		})
	}

	errorCases := []string{
		"`status` NOT IN (?)",
		"`status` IN (?, ?)",
		"`status` IN (1)",
		"`status` IN (SELECT `id` FROM `item`)",
		"`title` LIKE 'a%'",
		"`unknown` = ?",
		"`id` + 1 = ?",
		"`id` = `status`",
		"`id` <=> ?",
		"`id` BETWEEN ? AND ?",
		"`id` = 1.5",
		"NOT `id` = ?",
	}
	for _, where := range errorCases {
		t.Run(where, func(t *testing.T) {
			_, err := newWhereNode(m, parse(t, where))
			assert.Error(t, err)
		})
	}
}
