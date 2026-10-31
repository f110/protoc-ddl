package dao

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
