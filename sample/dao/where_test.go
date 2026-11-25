package dao

import (
	"context"
	"database/sql"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestWhereCondition(t *testing.T) {
	cases := []struct {
		name     string
		cond     *whereCondition
		wantSQL  string
		wantArgs []any
	}{
		{
			name:     "leaf",
			cond:     whereCond("`title` LIKE ?", "%foo%"),
			wantSQL:  "`title` LIKE ?",
			wantArgs: []any{"%foo%"},
		},
		{
			name:     "in expands the placeholders",
			cond:     whereIn("`status`", []uint32{1, 2}),
			wantSQL:  "`status` IN (?, ?)",
			wantArgs: []any{uint32(1), uint32(2)},
		},
		{
			name:    "empty in is false",
			cond:    whereIn("`status`", []uint32{}),
			wantSQL: "1 = 0",
		},
		{
			name:     "and",
			cond:     whereAnd(whereIn("`status`", []uint32{1}), whereCond("`id` > ?", 10)),
			wantSQL:  "(`status` IN (?) AND `id` > ?)",
			wantArgs: []any{uint32(1), 10},
		},
		{
			name:     "empty in in and",
			cond:     whereAnd(whereIn("`status`", []uint32(nil)), whereCond("`id` > ?", 10)),
			wantSQL:  "(1 = 0 AND `id` > ?)",
			wantArgs: []any{10},
		},
		{
			name:     "empty in in or",
			cond:     whereAnd(whereCond("`a` = ?", 1), whereOr(whereCond("`title` LIKE ?", "%x%"), whereIn("`id`", []int32{}))),
			wantSQL:  "(`a` = ? AND (`title` LIKE ? OR 1 = 0))",
			wantArgs: []any{1, "%x%"},
		},
		{
			name:     "nested",
			cond:     whereAnd(whereCond("`a` = ?", 1), whereOr(whereCond("`b` = ?", 2), whereCond("`c` = ?", 3))),
			wantSQL:  "(`a` = ? AND (`b` = ? OR `c` = ?))",
			wantArgs: []any{1, 2, 3},
		},
		{
			name:    "all in are empty",
			cond:    whereAnd(whereIn("`a`", []int32{}), whereOr(whereIn("`b`", []int32{}), whereIn("`c`", []int32{}))),
			wantSQL: "(1 = 0 AND (1 = 0 OR 1 = 0))",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			assert.Equal(t, tc.wantSQL, tc.cond.sql)
			assert.Equal(t, tc.wantArgs, tc.cond.args)
		})
	}
}

type recordConn struct {
	queries []string
	args    [][]any
}

func (c *recordConn) ExecContext(_ context.Context, query string, args ...any) (sql.Result, error) {
	c.queries = append(c.queries, query)
	c.args = append(c.args, args)
	return nil, nil
}

func TestInsertMulti(t *testing.T) {
	t.Run("placeholders", func(t *testing.T) {
		rows := make([][]any, maxPlaceholders+1)
		for i := range rows {
			rows[i] = []any{i}
		}
		conn := &recordConn{}
		require.NoError(t, insertMulti(context.Background(), conn, "INSERT INTO `t` (`a`) VALUES ", "(?)", rows))
		require.Len(t, conn.queries, 2)
		assert.Len(t, conn.args[0], maxPlaceholders)
		assert.Len(t, conn.args[1], 1)
		assert.Equal(t, "INSERT INTO `t` (`a`) VALUES (?)", conn.queries[1])
	})

	t.Run("size", func(t *testing.T) {
		payload := make([]byte, maxStatementSize/3)
		rows := [][]any{{1, payload}, {2, payload}, {3, &payload}}
		conn := &recordConn{}
		require.NoError(t, insertMulti(context.Background(), conn, "INSERT INTO `t` (`a`, `b`) VALUES ", "(?, ?)", rows))
		require.Len(t, conn.queries, 3)
		for _, v := range conn.args {
			assert.Len(t, v, 2)
		}
	})

	t.Run("multiple rows", func(t *testing.T) {
		conn := &recordConn{}
		require.NoError(t, insertMulti(context.Background(), conn, "INSERT INTO `t` (`a`, `b`) VALUES ", "(?, ?)", [][]any{{1, "x"}, {2, "y"}}))
		require.Len(t, conn.queries, 1)
		assert.Equal(t, "INSERT INTO `t` (`a`, `b`) VALUES (?, ?), (?, ?)", conn.queries[0])
		assert.Equal(t, []any{1, "x", 2, "y"}, conn.args[0])
	})
}
