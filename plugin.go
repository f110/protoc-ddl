package genddl

import (
	"bytes"
	"io/ioutil"
	"log"
	"os"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/golang/protobuf/protoc-gen-go/plugin"
)

type Column struct {
	Name     string
	DataType descriptor.FieldDescriptorProto_Type
	TypeName string
	Size     int
	Null     bool
	Sequence bool
	Default  string
}

type Table struct {
	Name       string
	Columns    []Column
	PrimaryKey []string
	Indexes    []*IndexOption
	Engine     string
}

func ParseInput() (*plugin_go.CodeGeneratorRequest, error) {
	buf, err := ioutil.ReadAll(os.Stdin)
	if err != nil {
		return nil, err
	}
	var input plugin_go.CodeGeneratorRequest
	err = proto.Unmarshal(buf, &input)
	if err != nil {
		return nil, err
	}

	return &input, nil
}

func Process(req *plugin_go.CodeGeneratorRequest) *plugin_go.CodeGeneratorResponse {
	tables := make([]Table, 0)

	files := make(map[string]*descriptor.FileDescriptorProto)
	for _, f := range req.ProtoFile {
		files[f.GetName()] = f
	}

	for _, fileName := range req.FileToGenerate {
		f := files[fileName]
		for _, m := range f.GetMessageType() {
			opt := m.GetOptions()
			e, err := proto.GetExtension(opt, E_Table)
			if err == proto.ErrMissingExtension {
				continue
			}
			ext := e.(*TableOptions)

			tables = append(tables, convertToTable(m, ext))
		}
	}

	var res plugin_go.CodeGeneratorResponse
	var buf bytes.Buffer

	MySQLDialect{}.Generate(&buf, tables)

	//res.File = append(res.File, &plugin_go.CodeGeneratorResponse_File{
	//	Name:    proto.String("sample/schema.sql"),
	//	Content: proto.String(buf.String()),
	//})
	log.Print(buf.String())

	return &res
}

func convertToTable(msg *descriptor.DescriptorProto, opt *TableOptions) Table {
	t := Table{}

	if opt.TableName != "" {
		t.Name = opt.TableName
	} else {
		t.Name = strings.ToLower(msg.GetName())
	}

	if len(opt.PrimaryKey) > 0 {
		t.PrimaryKey = opt.PrimaryKey
	}

	for _, f := range msg.GetField() {
		t.Columns = append(t.Columns, convertToColumn(f))
	}

	t.Indexes = opt.GetIndexes()
	t.Engine = opt.Engine

	return t
}

func convertToColumn(field *descriptor.FieldDescriptorProto) Column {
	f := Column{}

	f.Name = field.GetName()
	f.DataType = field.GetType()
	f.TypeName = field.GetTypeName()

	opt := field.GetOptions()
	if opt == nil {
		return f
	}
	e, err := proto.GetExtension(opt, E_Column)
	if err == proto.ErrMissingExtension {
		return f
	}
	ext := e.(*ColumnOptions)
	f.Sequence = ext.Sequence
	f.Null = ext.Null
	f.Default = ext.Default
	f.Size = int(ext.Size)
	f.TypeName = ext.Type

	return f
}
