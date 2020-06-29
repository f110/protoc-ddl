package generator

import (
	"bytes"
	"fmt"
	"go/format"
	"log"
	"path/filepath"
	"strings"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"

	"go.f110.dev/protoc-ddl/internal/schema"
)

const GoEntityGeneratorVersion = "v0.1"

type GoEntityGenerator struct{}

var (
	GoDataTypeMap = map[string]string{
		"TYPE_FLOAT":         "float32",
		"TYPE_DOUBLE":        "float64",
		"TYPE_INT32":         "int32",
		"TYPE_INT64":         "int64",
		"TYPE_UINT32":        "uint32",
		"TYPE_UINT64":        "uint64",
		"TYPE_SINT32":        "int",
		"TYPE_SINT64":        "int64",
		"TYPE_FIXED32":       "uint32",
		"TYPE_FIXED64":       "uint64",
		"TYPE_SFIXED32":      "int",
		"TYPE_SFIXED64":      "int64",
		"TYPE_BOOL":          "bool",
		"TYPE_BYTES":         "[]byte",
		"TYPE_STRING":        "string",
		schema.TimestampType: "time.Time",
	}
)

var importPackages = []string{"time", "bytes", "sync"}
var thirdPartyPackages = []string{"go.f110.dev/protoc-ddl"}

func (GoEntityGenerator) Generate(buf *bytes.Buffer, fileOpt *descriptor.FileOptions, messages *schema.Messages) {
	src := new(bytes.Buffer)

	packageName := fileOpt.GetGoPackage()
	if strings.Contains(packageName, ";") {
		s := strings.SplitN(packageName, ";", 2)
		packageName = s[1]
	} else {
		packageName = filepath.Base(packageName)
	}
	src.WriteString(fmt.Sprintf("package %s\n", packageName))
	src.WriteString("import (\n")
	for _, v := range importPackages {
		src.WriteString("\"" + v + "\"\n")
	}
	src.WriteRune('\n')
	for _, v := range thirdPartyPackages {
		src.WriteString("\"" + v + "\"\n")
	}
	src.WriteString(")\n")
	src.WriteString("var _ = time.Time{}\n")
	src.WriteString("var _ = bytes.Buffer{}\n")
	src.WriteRune('\n')
	src.WriteString("type Column struct {\n")
	src.WriteString("Name string\n")
	src.WriteString("Value interface{}\n")
	src.WriteString("}\n")
	src.WriteRune('\n')

	messages.Each(func(m *schema.Message) {
		src.WriteString(fmt.Sprintf("type %s struct {\n", m.Descriptor.GetName()))
		m.Fields.Each(func(f *schema.Field) {
			null := ""
			if f.Null {
				null = "*"
			}
			src.WriteString(fmt.Sprintf("%s %s%s\n", schema.ToCamel(f.Name), null, GoDataTypeMap[f.Type]))
		})
		src.WriteRune('\n')
		for _, v := range m.Descriptor.Field {
			if v.GetType() == descriptor.FieldDescriptorProto_TYPE_MESSAGE && v.GetTypeName() != schema.TimestampType {
				s := strings.Split(v.GetTypeName(), ".")
				src.WriteString(fmt.Sprintf("%s *%s\n", schema.ToCamel(v.GetName()), s[len(s)-1]))
			}
		}
		src.WriteString("\n")
		src.WriteString("mu sync.Mutex\n")
		src.WriteString(fmt.Sprintf("mark *%s\n", m.Descriptor.GetName()))
		src.WriteString("}\n\n")

		// ResetMark()
		src.WriteString(fmt.Sprintf("func (e *%s) ResetMark() {\n", m.Descriptor.GetName()))
		src.WriteString("e.mu.Lock()\n")
		src.WriteString("defer e.mu.Unlock()\n")
		src.WriteRune('\n')
		src.WriteString("e.mark = e.Copy()\n")
		src.WriteString("}\n\n")

		// IsChanged() bool
		src.WriteString(fmt.Sprintf("func (e *%s) IsChanged() bool {\n", m.Descriptor.GetName()))
		expr := make([]string, 0)
		m.Fields.Each(func(f *schema.Field) {
			if m.IsPrimaryKey(f) {
				return
			}

			null := ""
			if f.Null {
				null = "*"
			}
			switch f.Type {
			case "TYPE_BYTES":
				expr = append(expr, fmt.Sprintf("!bytes.Equal(e.%s, e.mark.%s)", schema.ToCamel(f.Name), schema.ToCamel(f.Name)))
			case schema.TimestampType:
				expr = append(expr, fmt.Sprintf("!e.%s.Equal(%se.mark.%s)", schema.ToCamel(f.Name), null, schema.ToCamel(f.Name)))
			default:
				expr = append(expr, fmt.Sprintf("%se.%s != %se.mark.%s", null, schema.ToCamel(f.Name), null, schema.ToCamel(f.Name)))
			}
		})
		src.WriteString("e.mu.Lock()\n")
		src.WriteString("e.mu.Unlock()\n")
		src.WriteRune('\n')
		if len(expr) > 0 {
			src.WriteString(fmt.Sprintf("return %s\n", strings.Join(expr, " || \n")))
		} else {
			src.WriteString("return false\n")
		}
		src.WriteString("}\n\n")

		// ChangedColumn() []ddl.Column
		src.WriteString(fmt.Sprintf("func (e *%s) ChangedColumn() []ddl.Column {\n", m.Descriptor.GetName()))
		src.WriteString("e.mu.Lock()\n")
		src.WriteString("e.mu.Unlock()\n")
		src.WriteRune('\n')
		src.WriteString("res := make([]ddl.Column, 0)\n")
		m.Fields.Each(func(f *schema.Field) {
			if m.IsPrimaryKey(f) {
				return
			}

			null := ""
			if f.Null {
				null = "*"
				src.WriteString(fmt.Sprintf("if e.%s != nil {\n", schema.ToCamel(f.Name)))
			}
			switch f.Type {
			case "TYPE_BYTES":
				src.WriteString(fmt.Sprintf("if !bytes.Equal(e.%s, e.mark.%s) {\n", schema.ToCamel(f.Name), schema.ToCamel(f.Name)))
			case schema.TimestampType:
				src.WriteString(fmt.Sprintf("if !e.%s.Equal(%se.mark.%s) {\n", schema.ToCamel(f.Name), null, schema.ToCamel(f.Name)))
			default:
				src.WriteString(fmt.Sprintf("if %se.%s != %se.mark.%s {\n", null, schema.ToCamel(f.Name), null, schema.ToCamel(f.Name)))
			}
			src.WriteString(fmt.Sprintf("res = append(res, ddl.Column{Name:\"%s\",Value:%se.%s})\n", schema.ToSnake(f.Name), null, schema.ToCamel(f.Name)))
			src.WriteString("}\n")
			if f.Null {
				src.WriteString("}\n")
			}
		})
		src.WriteRune('\n')
		src.WriteString("return res\n")
		src.WriteString("}\n")

		// Copy() *Entity
		src.WriteString(fmt.Sprintf("func (e *%s) Copy() *%s {\n", m.Descriptor.GetName(), m.Descriptor.GetName()))
		src.WriteString(fmt.Sprintf("n := &%s{\n", m.Descriptor.GetName()))
		m.Fields.Each(func(f *schema.Field) {
			if f.Null {
				return
			}
			src.WriteString(fmt.Sprintf("%s: e.%s,\n", schema.ToCamel(f.Name), schema.ToCamel(f.Name)))
		})
		src.WriteString("}\n")
		m.Fields.Each(func(f *schema.Field) {
			if !f.Null {
				return
			}
			src.WriteString(fmt.Sprintf("if e.%s != nil {\n", schema.ToCamel(f.Name)))
			src.WriteString(fmt.Sprintf("v := *e.%s\n", schema.ToCamel(f.Name)))
			src.WriteString(fmt.Sprintf("n.%s = &v\n", schema.ToCamel(f.Name)))
			src.WriteString("}\n")
		})
		src.WriteRune('\n')
		src.WriteString("return n\n")
		src.WriteString("}\n")
	})

	buf.WriteString("// Generated by protoc-ddl.\n")
	buf.WriteString(fmt.Sprintf("// protoc-gen-entity: %s\n", GoEntityGeneratorVersion))
	b, err := format.Source(src.Bytes())
	if err != nil {
		log.Print(src.String())
		log.Print(err)
		return
	}
	buf.Write(b)
}
