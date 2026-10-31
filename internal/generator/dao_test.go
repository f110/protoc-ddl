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
