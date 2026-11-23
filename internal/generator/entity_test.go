package generator

import (
	"bytes"
	"testing"

	"github.com/stretchr/testify/require"
	"google.golang.org/protobuf/types/descriptorpb"

	"go.f110.dev/protoc-ddl/internal/schema"
)

func TestGoEntityGenerator_CountStruct(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	buf := new(bytes.Buffer)
	GoEntityGenerator{}.Generate(
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
					{Name: "category_id", Type: "TYPE_INT32", Null: true},
				}),
				SelectQueries: []*schema.Query{
					{Name: "List", Query: "SELECT * FROM `:table_name:`"},
				},
				CountQueries: []*schema.Query{
					{Name: "ByStatus", Query: "SELECT `status`, `category_id`, COUNT(*) FROM `:table_name:` GROUP BY `status`, `category_id`"},
					{Name: "All", Query: "SELECT COUNT(*) FROM `:table_name:`"},
				},
			},
			{
				Descriptor:  &descriptorpb.DescriptorProto{Name: new("Tag")},
				TableName:   "tag",
				PrimaryKeys: []*schema.Field{pk},
				Fields:      schema.NewFields([]*schema.Field{pk}),
			},
		}, nil),
	)
	out := buf.String()

	require.Contains(t, out, "// ItemByStatusCount is the result of CountByStatus.\ntype ItemByStatusCount struct {\n\tStatus     uint32\n\tCategoryId *int32\n\tCount      int64\n}\n\ntype Tag struct {\n")
	require.NotContains(t, out, "ItemAllCount")
	require.NotContains(t, out, "ItemListCount")
}

func TestGoEntityGenerator_InvalidSource(t *testing.T) {
	pk := &schema.Field{Name: "id", Type: "TYPE_INT32", Sequence: true}
	err := GoEntityGenerator{}.Generate(
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
	)
	require.Error(t, err)
}
