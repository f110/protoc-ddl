package generator

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/pingcap/tidb/pkg/parser"
	"github.com/pingcap/tidb/pkg/parser/ast"
	_ "github.com/pingcap/tidb/pkg/parser/test_driver"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.f110.dev/protoc-ddl/internal/schema"
)

func TestGoDAOGenerator_Generate(t *testing.T) {
	res := new(bytes.Buffer)
	primaryKey := &schema.Field{
		Name:     "id",
		Type:     "TYPE_INT32",
		Sequence: true,
	}
	GoDAOGenerator{}.Generate(
		res,
		&descriptorpb.FileOptions{
			GoPackage: new("go.f110.dev/protoc-ddl/test/database"),
		},
		schema.NewMessages([]*schema.Message{
			{
				Descriptor: &descriptorpb.DescriptorProto{
					Name: new("User"),
				},
				TableName:   "user",
				PrimaryKeys: []*schema.Field{primaryKey},
				Fields: schema.NewFields([]*schema.Field{
					primaryKey,
					{
						Name: "name",
						Type: "TYPE_STRING",
					},
					{
						Name: "age",
						Type: "TYPE_INT32",
					},
				}),
				SelectQueries: []*schema.Query{
					{
						Name:  "OverTwenty",
						Query: "SELECT * FROM user WHERE age > 20",
					},
					{
						Name:  "Name",
						Query: "SELECT name FROM user WHERE id = ? AND name = ?",
					},
					{
						Name:  "UniqUname",
						Query: "SELECT distinct(name) FROM user",
					},
				},
			},
		}, nil),
	)

	t.Log(res.String())
}

func TestGoDAOGenerator_UpdateColumnSlice(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	gen := func(withTimestamp bool, fields []*schema.Field) string {
		buf := new(bytes.Buffer)
		GoDAOGenerator{}.Generate(
			buf,
			&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
			schema.NewMessages([]*schema.Message{
				{
					Descriptor:    &descriptorpb.DescriptorProto{Name: new("Item")},
					TableName:     "item",
					PrimaryKeys:   []*schema.Field{pk},
					Fields:        schema.NewFields(fields),
					WithTimestamp: withTimestamp,
				},
			}, nil),
		)
		return buf.String()
	}

	// A table without timestamps must size the UPDATE column slice exactly to the
	// changed columns. A stray +1 leaves an empty trailing element that renders as
	// "SET `col` = ?,  WHERE ...", which is invalid SQL and makes every Update fail.
	t.Run("without timestamp", func(t *testing.T) {
		out := gen(false, []*schema.Field{pk, {Name: "name", Type: "TYPE_STRING"}})
		require.Contains(t, out, "cols := make([]string, len(changedColumn))")
		require.NotContains(t, out, "len(changedColumn)+1")
		require.NotContains(t, out, "`updated_at` = ?")
	})

	// A table with timestamps reserves the extra slot for the auto-managed
	// updated_at column.
	t.Run("with timestamp", func(t *testing.T) {
		out := gen(true, []*schema.Field{
			pk,
			{Name: "name", Type: "TYPE_STRING"},
			{Name: "created_at", Type: schema.TimestampType},
			{Name: "updated_at", Type: schema.TimestampType, Null: true},
		})
		require.Contains(t, out, "cols := make([]string, len(changedColumn)+1)")
		require.Contains(t, out, "cols[len(cols)-1] = \"`updated_at` = ?\"")
	})
}

func TestGoDAOGenerator_SelectMultiRangeLoop(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	buf := new(bytes.Buffer)
	GoDAOGenerator{}.Generate(
		buf,
		&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
		schema.NewMessages([]*schema.Message{
			{
				Descriptor:  &descriptorpb.DescriptorProto{Name: new("Item")},
				TableName:   "item",
				PrimaryKeys: []*schema.Field{pk},
				Fields:      schema.NewFields([]*schema.Field{pk, {Name: "name", Type: "TYPE_STRING"}}),
			},
		}, nil),
	)

	out := buf.String()
	require.Contains(t, out, "for i := range id {")
	require.NotContains(t, out, "i < len(id)")
}

func TestGoDAOGenerator_RelationOrderIsStable(t *testing.T) {
	gen := func() string {
		pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
		parentPK := &schema.Field{Name: "id", Type: "TYPE_INT32", Descriptor: &descriptorpb.FieldDescriptorProto{Name: new("id")}}
		relations := schema.Relations{}
		fields := []*schema.Field{pk}
		for _, name := range []string{"alpha", "bravo", "charlie", "delta"} {
			ref := &schema.Field{Name: name + "_id", Type: "TYPE_INT32", Descriptor: parentPK.Descriptor}
			relations[&schema.Field{Name: name, Type: ".database.User"}] = []*schema.Field{ref}
			fields = append(fields, ref)
		}

		buf := new(bytes.Buffer)
		GoDAOGenerator{}.Generate(
			buf,
			&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
			schema.NewMessages([]*schema.Message{
				{
					Descriptor:  &descriptorpb.DescriptorProto{Name: new("User")},
					TableName:   "user",
					PrimaryKeys: []*schema.Field{parentPK},
					Fields:      schema.NewFields([]*schema.Field{parentPK, {Name: "name", Type: "TYPE_STRING"}}),
				},
				{
					Descriptor:    &descriptorpb.DescriptorProto{Name: new("Item")},
					TableName:     "item",
					PrimaryKeys:   []*schema.Field{pk},
					Fields:        schema.NewFields(fields),
					Relations:     relations,
					SelectQueries: []*schema.Query{{Name: "All", Query: "SELECT * FROM item"}},
				},
			}, nil),
		)
		return buf.String()
	}

	expected := gen()
	for range 20 {
		require.Equal(t, expected, gen())
	}
}

func TestGoDAOStruct_findArgs(t *testing.T) {
	tableFields := []*schema.Field{
		{
			Name:     "id",
			Sequence: true,
			Type:     "TYPE_INT32",
		},
		{
			Name: "age",
			Type: "TYPE_INT32",
		},
		{
			Name: "name",
			Type: "TYPE_STRING",
		},
	}
	fieldMap := make(map[string]*schema.Field)
	for _, v := range tableFields {
		fieldMap[v.Name] = v
	}

	tests := []struct {
		Name   string
		Where  string
		Fields []*schema.Field
	}{
		{
			Name:   "constant value",
			Where:  "age > 20",
			Fields: nil,
		},
		{
			Name:   "Single field",
			Where:  "age = ?",
			Fields: []*schema.Field{fieldMap["age"]},
		},
		{
			Name:   "Multi product set fields",
			Where:  "name = ? AND age = ?",
			Fields: []*schema.Field{fieldMap["name"], fieldMap["age"]},
		},
		{
			Name:   "Multi union fields",
			Where:  "name = ? OR age = ?",
			Fields: []*schema.Field{fieldMap["name"], fieldMap["age"]},
		},
		{
			Name:   "is operator",
			Where:  "`start_at` IS NULL",
			Fields: nil,
		},
	}

	s := &GoDAOStruct{}
	p := parser.New()
	for _, v := range tests {
		tt := v
		t.Run(tt.Where, func(t *testing.T) {
			stmt, w, err := p.Parse("SELECT * FROM tmp WHERE "+tt.Where, "", "")
			require.NoError(t, err)
			require.Len(t, w, 0)
			sel := stmt[0].(*ast.SelectStmt)

			fields := s.findArgs(tableFields, sel.Where)
			if tt.Fields == nil {
				require.Nil(t, fields, "Expect no field")
			}
			require.Len(t, fields, len(tt.Fields), "Expect the number of fields is %d", len(tt.Fields))

			for i, f := range tt.Fields {
				if fields[i] != f {
					t.Errorf("Expect %s got %s", f.Name, fields[i].Name)
				}
			}
		})
	}
}

func TestGoFunc(t *testing.T) {
	f := &goFunc{
		Name: "Tx",
		Args: fieldList{
			{
				Name: "fn",
				Type: fmt.Sprint(&goFunc{
					Args:    fieldList{{Name: "tx", Type: "sql.Tx", Pointer: true}},
					Returns: fieldList{{Type: "error"}},
				}),
			},
		},
		Returns: fieldList{{Type: "error"}},
		Body:    "return nil",
	}
	t.Log(f.String())
}

func TestGoDAOGenerator_ConditionQuery(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	gen := func(queries ...*schema.Query) string {
		buf := new(bytes.Buffer)
		GoDAOGenerator{}.Generate(
			buf,
			&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
			schema.NewMessages([]*schema.Message{
				{
					Descriptor:  &descriptorpb.DescriptorProto{Name: new("Item")},
					TableName:   "item",
					PrimaryKeys: []*schema.Field{pk},
					Fields: schema.NewFields([]*schema.Field{
						pk,
						{Name: "status", Type: "TYPE_UINT32"},
						{Name: "title", Type: "TYPE_STRING"},
						{Name: "body", Type: "TYPE_STRING"},
						{Name: "deleted_at", Type: schema.TimestampType, Null: true},
					}),
					SelectQueries: queries,
				},
			}, nil),
		)
		return buf.String()
	}

	t.Run("in and like are built at runtime", func(t *testing.T) {
		out := gen(&schema.Query{
			Name:  "Search",
			Query: "SELECT * FROM `:table_name:` WHERE `status` IN (?) AND (`title` LIKE ? OR `body` LIKE ? OR `id` IN (?)) AND `deleted_at` IS NULL",
		})
		require.NotContains(t, out, `"go.f110.dev/protoc-ddl"`)
		require.Contains(t, out, "ListSearch(ctx context.Context, status []uint32, title string, body string, id []int32, opt ...ListOption) ([]*database.Item, error)")
		require.Contains(t, out, "where := whereAnd(whereIn(\"`status`\", status), whereOr(whereCond(\"`title` LIKE ?\", title), whereCond(\"`body` LIKE ?\", body), whereIn(\"`id`\", id)), whereCond(\"`deleted_at` IS NULL\"))")
		require.Contains(t, out, "query := \"SELECT `id`, `status`, `title`, `body`, `deleted_at` FROM `item` WHERE \" + where.sql")
		require.Contains(t, out, "rows, err := d.conn.QueryContext(ctx, query, where.args...)")
		require.Contains(t, out, "func whereIn[T any](column string, values []T) *whereCondition {")
	})

	t.Run("same column is used twice", func(t *testing.T) {
		out := gen(&schema.Query{
			Name:  "Title",
			Query: "SELECT * FROM `:table_name:` WHERE `title` LIKE ? OR `title` = ?",
		})
		require.Contains(t, out, "ListTitle(ctx context.Context, title string, title2 string, opt ...ListOption)")
	})

	t.Run("query without in, like and parentheses is not changed", func(t *testing.T) {
		out := gen(&schema.Query{Name: "ByStatus", Query: "SELECT * FROM `:table_name:` WHERE `status` = ?"})
		require.NotContains(t, out, "whereCondition")
		require.Contains(t, out, "ListByStatus(ctx context.Context, status uint32, opt ...ListOption)")
	})
}

func TestGoDAOGenerator_SelectSingleRowColumns(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	hash := &schema.Field{Name: "hash", Type: "TYPE_STRING"}
	buf := new(bytes.Buffer)
	err := GoDAOGenerator{}.Generate(
		buf,
		&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
		schema.NewMessages([]*schema.Message{
			{
				Descriptor:  &descriptorpb.DescriptorProto{Name: new("Item")},
				TableName:   "item",
				PrimaryKeys: []*schema.Field{pk},
				Fields:      schema.NewFields([]*schema.Field{pk, hash, {Name: "encoding", Type: "TYPE_INT32"}, {Name: "size", Type: "TYPE_INT64"}}),
				Indexes:     []*schema.Index{{Name: "idx_hash", Columns: schema.NewFields([]*schema.Field{hash}), Unique: true}},
				SelectQueries: []*schema.Query{
					{Name: "SizeByHash", Query: "SELECT `encoding`, `size` FROM `:table_name:` WHERE `hash` = ?"},
					{Name: "ByHash", Query: "SELECT * FROM `:table_name:` WHERE `hash` = ?"},
				},
			},
		}, nil),
	)
	require.NoError(t, err)
	out := buf.String()
	require.Contains(t, out, "SelectSizeByHash(ctx context.Context, hash string) (*database.Item, error)")
	require.Contains(t, out, "if err := row.Scan(&v.Encoding, &v.Size); err != nil {")
	require.Contains(t, out, "SelectByHash(ctx context.Context, hash string) (*database.Item, error)")
	require.Contains(t, out, "if err := row.Scan(&v.Id, &v.Hash, &v.Encoding, &v.Size); err != nil {")
}

func TestGoDAOGenerator_CountQuery(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	gen := func(queries ...*schema.Query) string {
		m := &schema.Message{
			Descriptor:  &descriptorpb.DescriptorProto{Name: new("Item")},
			TableName:   "item",
			PrimaryKeys: []*schema.Field{pk},
			Fields: schema.NewFields([]*schema.Field{
				pk,
				{Name: "status", Type: "TYPE_UINT32"},
				{Name: "title", Type: "TYPE_STRING"},
				{Name: "deleted_at", Type: schema.TimestampType, Null: true},
			}),
		}
		for _, q := range queries {
			m.AddQuery(q.Name, q.Query)
		}
		buf := new(bytes.Buffer)
		GoDAOGenerator{}.Generate(
			buf,
			&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
			schema.NewMessages([]*schema.Message{m}, nil),
		)
		return buf.String()
	}

	t.Run("count with group by returns the slice", func(t *testing.T) {
		out := gen(&schema.Query{
			Name:  "ActiveByIds",
			Query: "SELECT `status`, COUNT(*) FROM `:table_name:` WHERE `id` IN (?) AND `deleted_at` IS NULL GROUP BY `status`",
		})
		require.NotContains(t, out, "ListActiveByIds")
		require.Contains(t, out, "CountActiveByIds(ctx context.Context, id []int32) ([]*database.ItemActiveByIdsCount, error)")
		require.Contains(t, out, "where := whereAnd(whereIn(\"`id`\", id), whereCond(\"`deleted_at` IS NULL\"))")
		require.Contains(t, out, "query := \"SELECT `status`, COUNT(*) FROM `item` WHERE \" + where.sql + \" GROUP BY `status`\"")
		require.Contains(t, out, "rows, err := d.conn.QueryContext(ctx, query, where.args...)")
		require.Contains(t, out, "r := &database.ItemActiveByIdsCount{}")
		require.Contains(t, out, "if err := rows.Scan(&r.Status, &r.Count); err != nil {")
	})

	t.Run("count without group by returns the number", func(t *testing.T) {
		out := gen(&schema.Query{Name: "ByStatus", Query: "SELECT COUNT(*) FROM `:table_name:` WHERE `status` = ?"})
		require.NotContains(t, out, "ListByStatus")
		require.Contains(t, out, "CountByStatus(ctx context.Context, status uint32) (int64, error)")
		require.Contains(t, out, "query := \"SELECT COUNT(*) FROM `item` WHERE \" + where.sql")
		require.Contains(t, out, "if err := d.conn.QueryRowContext(ctx, query, where.args...).Scan(&count); err != nil {")
	})

	t.Run("count without where", func(t *testing.T) {
		out := gen(&schema.Query{Name: "All", Query: "SELECT COUNT(*) FROM `:table_name:`"})
		require.Contains(t, out, "CountAll(ctx context.Context) (int64, error)")
		require.Contains(t, out, "query := \"SELECT COUNT(*) FROM `item`\"")
		require.Contains(t, out, "if err := d.conn.QueryRowContext(ctx, query).Scan(&count); err != nil {")
	})

	t.Run("unsupported count is not generated", func(t *testing.T) {
		out := gen(
			&schema.Query{Name: "Column", Query: "SELECT COUNT(`title`) FROM `:table_name:`"},
			&schema.Query{Name: "Distinct", Query: "SELECT COUNT(DISTINCT `title`) FROM `:table_name:`"},
			&schema.Query{Name: "NotGrouped", Query: "SELECT `status`, COUNT(*) FROM `:table_name:`"},
			&schema.Query{Name: "Ordered", Query: "SELECT `status`, COUNT(*) FROM `:table_name:` GROUP BY `status` ORDER BY `status`"},
		)
		for _, name := range []string{"Column", "Distinct", "NotGrouped", "Ordered"} {
			require.NotContains(t, out, "Count"+name+"(")
			require.NotContains(t, out, "List"+name+"(")
		}
	})
}

func TestGoDAOGenerator_ExprQuery(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	gen := func(queries ...*schema.Query) string {
		m := &schema.Message{
			Descriptor:  &descriptorpb.DescriptorProto{Name: new("Item")},
			TableName:   "item",
			PrimaryKeys: []*schema.Field{pk},
			Fields: schema.NewFields([]*schema.Field{
				pk,
				{Name: "status", Type: "TYPE_UINT32"},
				{Name: "body", Type: "TYPE_BYTES"},
				{Name: "note", Type: "TYPE_STRING", Null: true},
			}),
		}
		for _, q := range queries {
			m.AddQuery(q.Name, q.Query)
		}
		buf := new(bytes.Buffer)
		err := GoDAOGenerator{}.Generate(
			buf,
			&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
			schema.NewMessages([]*schema.Message{m}, nil),
		)
		require.NoError(t, err)
		return buf.String()
	}

	t.Run("expression with where returns the slice of the row", func(t *testing.T) {
		out := gen(&schema.Query{
			Name:  "BodySize",
			Query: "SELECT `id`, `status`, LENGTH(`body`) AS `body_size` FROM `:table_name:` WHERE `status` IN (?)",
		})
		require.Contains(t, out, "ListBodySize(ctx context.Context, status []uint32, opt ...ListOption) ([]*database.ItemBodySizeRow, error)")
		require.Contains(t, out, "where := whereIn(\"`status`\", status)")
		require.Contains(t, out, "query := \"SELECT `id`, `status`, LENGTH(`body`) AS `body_size` FROM `item` WHERE \" + where.sql")
		require.Contains(t, out, "rows, err := d.conn.QueryContext(ctx, query, where.args...)")
		require.Contains(t, out, "r := &database.ItemBodySizeRow{}")
		require.Contains(t, out, "if err := rows.Scan(&r.Id, &r.Status, &r.BodySize); err != nil {")
	})

	t.Run("expression without where", func(t *testing.T) {
		out := gen(&schema.Query{Name: "NoteSize", Query: "SELECT CHAR_LENGTH(`note`) AS `note_size` FROM `:table_name:`"})
		require.Contains(t, out, "ListNoteSize(ctx context.Context, opt ...ListOption) ([]*database.ItemNoteSizeRow, error)")
		require.Contains(t, out, "query := \"SELECT CHAR_LENGTH(`note`) AS `note_size` FROM `item`\"")
		require.Contains(t, out, "rows, err := d.conn.QueryContext(ctx, query)")
		require.Contains(t, out, "if err := rows.Scan(&r.NoteSize); err != nil {")
	})

	t.Run("expression which returns the string", func(t *testing.T) {
		out := gen(&schema.Query{Name: "UpperNote", Query: "SELECT `id`, UPPER(`note`) AS `upper_note` FROM `:table_name:`"})
		require.Contains(t, out, "ListUpperNote(ctx context.Context, opt ...ListOption) ([]*database.ItemUpperNoteRow, error)")
		require.Contains(t, out, "query := \"SELECT `id`, UPPER(`note`) AS `upper_note` FROM `item`\"")
		require.Contains(t, out, "if err := rows.Scan(&r.Id, &r.UpperNote); err != nil {")
	})

	t.Run("unsupported expression is not generated", func(t *testing.T) {
		out := gen(
			&schema.Query{Name: "NoAlias", Query: "SELECT LENGTH(`body`) FROM `:table_name:`"},
			&schema.Query{Name: "UnknownFunc", Query: "SELECT ROUND(`status`) AS `rounded` FROM `:table_name:`"},
			&schema.Query{Name: "NotColumn", Query: "SELECT LENGTH('foo') AS `size` FROM `:table_name:`"},
			&schema.Query{Name: "UnknownColumn", Query: "SELECT LENGTH(`foo`) AS `size` FROM `:table_name:`"},
			&schema.Query{Name: "Wildcard", Query: "SELECT *, LENGTH(`body`) AS `body_size` FROM `:table_name:`"},
			&schema.Query{Name: "Ordered", Query: "SELECT LENGTH(`body`) AS `body_size` FROM `:table_name:` ORDER BY `id`"},
		)
		for _, name := range []string{"NoAlias", "UnknownFunc", "NotColumn", "UnknownColumn", "Wildcard", "Ordered"} {
			require.NotContains(t, out, "List"+name+"(")
			require.NotContains(t, out, "Select"+name+"(")
		}
	})
}

func TestGoDAOGenerator_GoIdentifierColumn(t *testing.T) {
	pk := &schema.Field{Name: "type", Type: "TYPE_INT32"}
	buf := new(bytes.Buffer)
	err := GoDAOGenerator{}.Generate(
		buf,
		&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
		schema.NewMessages([]*schema.Message{
			{
				Descriptor:  &descriptorpb.DescriptorProto{Name: new("Item")},
				TableName:   "item",
				PrimaryKeys: []*schema.Field{pk},
				Fields:      schema.NewFields([]*schema.Field{pk, {Name: "len", Type: "TYPE_UINT32"}}),
				SelectQueries: []*schema.Query{
					{Name: "ByLen", Query: "SELECT * FROM `:table_name:` WHERE `len` = ?"},
					{Name: "ByType", Query: "SELECT * FROM `:table_name:` WHERE `type` IN (?)"},
				},
			},
		}, nil),
	)
	require.NoError(t, err)
	out := buf.String()
	require.Contains(t, out, "Select(ctx context.Context, type_ int32)")
	require.Contains(t, out, "SelectMulti(ctx context.Context, type_ ...int32)")
	require.Contains(t, out, "ListByLen(ctx context.Context, len_ uint32, opt ...ListOption)")
	require.Contains(t, out, "ListByType(ctx context.Context, type_ []int32, opt ...ListOption)")
}

func TestGoDAOGenerator_InvalidSource(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	buf := new(bytes.Buffer)
	err := GoDAOGenerator{}.Generate(
		buf,
		&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
		schema.NewMessages([]*schema.Message{
			{
				Descriptor:  &descriptorpb.DescriptorProto{Name: new("Invalid Name")},
				TableName:   "item",
				PrimaryKeys: []*schema.Field{pk},
				Fields:      schema.NewFields([]*schema.Field{pk}),
			},
		}, nil),
	)
	require.Error(t, err)
}

func TestGoDAOMockGenerator_GoIdentifierColumn(t *testing.T) {
	pk := &schema.Field{Name: "type", Type: "TYPE_INT32"}
	buf := new(bytes.Buffer)
	err := GoDAOMockGenerator{}.Generate(
		buf,
		&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
		schema.NewMessages([]*schema.Message{
			{
				Descriptor:    &descriptorpb.DescriptorProto{Name: new("Item")},
				TableName:     "item",
				PrimaryKeys:   []*schema.Field{pk},
				Fields:        schema.NewFields([]*schema.Field{pk, {Name: "len", Type: "TYPE_UINT32"}}),
				SelectQueries: []*schema.Query{{Name: "ByLen", Query: "SELECT * FROM `:table_name:` WHERE `len` = ?"}},
			},
		}, nil),
		"go.f110.dev/protoc-ddl/test/database/dao",
	)
	require.NoError(t, err)
	out := buf.String()
	require.Contains(t, out, `d.Call("Select", map[string]any{"type_": type_})`)
	require.Contains(t, out, "RegisterSelect(type_ int32, value *database.Item)")
	require.Contains(t, out, "ListByLen(ctx context.Context, len_ uint32, opt ...dao.ListOption)")
}

func TestGoDAOMockGenerator_InvalidSource(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	err := GoDAOMockGenerator{}.Generate(
		new(bytes.Buffer),
		&descriptorpb.FileOptions{GoPackage: new("go.f110.dev/protoc-ddl/test/database")},
		schema.NewMessages([]*schema.Message{
			{
				Descriptor:  &descriptorpb.DescriptorProto{Name: new("Invalid Name")},
				TableName:   "item",
				PrimaryKeys: []*schema.Field{pk},
				Fields:      schema.NewFields([]*schema.Field{pk}),
			},
		}, nil),
		"go.f110.dev/protoc-ddl/test/database/dao",
	)
	require.Error(t, err)
}
