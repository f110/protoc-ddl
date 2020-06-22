package schema

import (
	"io"
	"io/ioutil"
	"regexp"
	"strings"

	"github.com/golang/protobuf/proto"
	"github.com/golang/protobuf/protoc-gen-go/descriptor"
	"github.com/golang/protobuf/protoc-gen-go/plugin"

	"go.f110.dev/protoc-ddl"
)

const (
	TimestampType = ".google.protobuf.Timestamp"
)

type Column struct {
	Name     string
	DataType string
	TypeName string
	Size     int
	Null     bool
	Sequence bool
	Default  string
}

type Table struct {
	Name           string
	Columns        []Column
	PrimaryKey     []string
	PrimaryKeyType string
	Indexes        []*ddl.IndexOption
	Engine         string
	WithTimestamp  bool

	packageName string
	descriptor  *descriptor.DescriptorProto
}

type Option struct {
	Dialect    string
	OutputFile string
}

func ParseInput(in io.Reader) (*plugin_go.CodeGeneratorRequest, error) {
	buf, err := ioutil.ReadAll(in)
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

func Process(req *plugin_go.CodeGeneratorRequest) (Option, []*Table) {
	tables := make([]*Table, 0)

	opt := parseOption(req.GetParameter())

	files := make(map[string]*descriptor.FileDescriptorProto)
	for _, f := range req.ProtoFile {
		files[f.GetName()] = f
	}

	targetMessages := make([]struct {
		Package    string
		Descriptor *descriptor.DescriptorProto
	}, 0)
	for _, fileName := range req.FileToGenerate {
		f := files[fileName]
		for _, m := range f.GetMessageType() {
			opt := m.GetOptions()
			_, err := proto.GetExtension(opt, ddl.E_Table)
			if err == proto.ErrMissingExtension {
				continue
			}

			targetMessages = append(targetMessages, struct {
				Package    string
				Descriptor *descriptor.DescriptorProto
			}{Package: f.GetPackage(), Descriptor: m})
		}
	}
	for _, m := range targetMessages {
		e, _ := proto.GetExtension(m.Descriptor.GetOptions(), ddl.E_Table)
		ext := e.(*ddl.TableOptions)

		tables = append(tables, convertToTable(m.Package, m.Descriptor, ext))
	}

	for _, t := range tables {
		for _, f := range t.descriptor.GetField() {
			t.Columns = append(t.Columns, convertToColumn(f, tables))
		}

		if t.WithTimestamp {
			t.Columns = append(t.Columns,
				Column{Name: "created_at", DataType: TimestampType},
				Column{Name: "updated_at", DataType: TimestampType, Null: true},
			)
		}
	}

	return opt, tables
}

func convertToTable(packageName string, msg *descriptor.DescriptorProto, opt *ddl.TableOptions) *Table {
	t := &Table{descriptor: msg, packageName: packageName, WithTimestamp: opt.WithTimestamp}

	if opt.TableName != "" {
		t.Name = opt.TableName
	} else {
		t.Name = ToSnake(msg.GetName())
	}

	if len(opt.PrimaryKey) > 0 {
		t.PrimaryKey = opt.PrimaryKey
		for _, f := range msg.GetField() {
			if f.GetName() == t.PrimaryKey[0] {
				t.PrimaryKeyType = f.GetType().String()
				break
			}
		}
	}

	t.Indexes = opt.GetIndexes()
	t.Engine = opt.Engine

	return t
}

func convertToColumn(field *descriptor.FieldDescriptorProto, tables []*Table) Column {
	f := Column{}

	f.Name = field.GetName()
	if field.GetType() == descriptor.FieldDescriptorProto_TYPE_MESSAGE {
		foreignTable := false
		for _, t := range tables {
			if "."+t.packageName+"."+t.descriptor.GetName() == field.GetTypeName() {
				foreignTable = true
				f.Name += "_" + t.PrimaryKey[0]
				f.DataType = t.PrimaryKeyType
			}
		}
		if !foreignTable {
			f.DataType = field.GetTypeName()
		}
	} else {
		f.DataType = field.GetType().String()
	}

	opt := field.GetOptions()
	if opt == nil {
		return f
	}
	e, err := proto.GetExtension(opt, ddl.E_Column)
	if err == proto.ErrMissingExtension {
		return f
	}
	ext := e.(*ddl.ColumnOptions)
	f.Sequence = ext.Sequence
	f.Null = ext.Null
	f.Default = ext.Default
	f.Size = int(ext.Size)
	f.TypeName = ext.Type

	return f
}

func parseOption(p string) Option {
	opt := Option{OutputFile: "sql/schema.sql"}
	params := strings.Split(p, ",")
	for _, param := range params {
		s := strings.SplitN(param, "=", 2)
		if len(s) == 1 {
			opt.OutputFile = s[0]
			continue
		}
		key := s[0]
		value := s[1]

		switch key {
		case "dialect":
			opt.Dialect = value
		}
	}
	return opt
}

var matchFirstCap = regexp.MustCompile("(.)([A-Z][a-z]+)")
var matchAllCap = regexp.MustCompile("([a-z0-9])([A-Z])")

func ToSnake(str string) string {
	snake := matchFirstCap.ReplaceAllString(str, "${1}_${2}")
	snake = matchAllCap.ReplaceAllString(snake, "${1}_${2}")
	return strings.ToLower(snake)
}
