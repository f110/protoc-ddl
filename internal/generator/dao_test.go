package generator

import (
	"bytes"
	"fmt"
	"testing"

	"github.com/pingcap/parser"
	"github.com/pingcap/parser/ast"
	_ "github.com/pingcap/parser/test_driver"
	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/proto"
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
			GoPackage: proto.String("go.f110.dev/protoc-ddl/test/database"),
		},
		schema.NewMessages([]*schema.Message{
			{
				Descriptor: &descriptorpb.DescriptorProto{
					Name: proto.String("User"),
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
				},
			},
		}, nil),
	)

	t.Log(res.String())
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
