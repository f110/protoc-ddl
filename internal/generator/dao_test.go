package generator

import (
	"bytes"
	"testing"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/stretchr/testify/assert"
	"vitess.io/vitess/go/vt/sqlparser"

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
		&descriptor.FileOptions{
			GoPackage: proto.String("go.f110.dev/protoc-ddl/test/database"),
		},
		schema.NewMessages([]*schema.Message{
			{
				Descriptor: &descriptor.DescriptorProto{
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
		}),
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
	}

	s := &GoDAOStruct{}
	for _, v := range tests {
		stmt, err := sqlparser.Parse("SELECT * FROM tmp WHERE " + v.Where)
		if err != nil {
			t.Fatal(err)
		}
		sel := stmt.(*sqlparser.Select)

		fields := s.findArgs(tableFields, sel.Where)
		if v.Fields == nil && fields != nil {
			t.Fatalf("%s: expect no field", v.Name)
		}
		if len(v.Fields) != len(fields) {
			t.Fatalf("Expect the number of fields is %d", len(v.Fields))
		}
		for i, f := range v.Fields {
			if fields[i] != f {
				t.Errorf("Expect %s got %s", f.Name, fields[i].Name)
			}
		}
	}
}

func TestPrintSelectQueryAST(t *testing.T) {
	cases := []struct {
		Query    string
		Rendered string
	}{
		{
			Query:    "SELECT * FROM `user`",
			Rendered: "select * from user",
		},
		{
			Query:    "select * FROM `user` WHERE id = ?",
			Rendered: "select * from user where id = ?",
		},
		{
			Query:    "SELECT `id`, `name` FROM `user` WHERE `id` = ?",
			Rendered: "select id, name from user where id = ?",
		},
	}

	for _, c := range cases {
		s, err := sqlparser.Parse(c.Query)
		if err != nil {
			t.Fatalf("%s: %v", c.Query, err)
		}
		stmt, ok := s.(*sqlparser.Select)
		if !ok {
			t.Fatal("Query is not select. This is a bug of test")
		}

		got := printSelectQueryAST(nil, stmt)
		assert.Equal(t, c.Rendered, got)
	}
}
