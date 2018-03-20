package genddl

import (
	"bytes"
	"fmt"
	"strconv"
	"strings"

	"github.com/golang/protobuf/protoc-gen-go/descriptor"
)

var (
	DataTypeMap = map[descriptor.FieldDescriptorProto_Type]string{
		descriptor.FieldDescriptorProto_TYPE_BOOL:   "TINYINT(1)",
		descriptor.FieldDescriptorProto_TYPE_BYTES:  "TINYINT",
		descriptor.FieldDescriptorProto_TYPE_INT32:  "INTEGER",
		descriptor.FieldDescriptorProto_TYPE_INT64:  "BIGINT",
		descriptor.FieldDescriptorProto_TYPE_UINT32: "INTEGER UNSIGNED",
		descriptor.FieldDescriptorProto_TYPE_UINT64: "BIGINT UNSIGNED",
		descriptor.FieldDescriptorProto_TYPE_SINT32: "INTEGER",
		descriptor.FieldDescriptorProto_TYPE_SINT64: "BIGINT",
		descriptor.FieldDescriptorProto_TYPE_FLOAT:  "FLOAT",
		descriptor.FieldDescriptorProto_TYPE_STRING: "VARCHAR",
	}
	ExtendTypeNameMap = map[string]string{
		".google.protobuf.Timestamp": "DATETIME",
	}
)

type MySQLDialect struct{}

func (m MySQLDialect) Generate(writer *bytes.Buffer, tables []Table) {
	writer.WriteString("SET foreign_key_checks=0;\n\n")
	writer.WriteString("")

	for _, table := range tables {
		writer.WriteString(fmt.Sprintf("DROP TABLE IF EXISTS `%s`;\n", table.Name))
		writer.WriteString(fmt.Sprintf("CREATE TABLE `%s` (\n", table.Name))

		colLine := make([]string, 0)
		for _, col := range table.Columns {
			nullConstraint := "NOT NULL"
			if col.Null {
				nullConstraint = "NULL"
			}
			columnOpt := ""
			if col.Sequence {
				columnOpt += " AUTO_INCREMENT"
			}
			if col.Default != "" {
				columnOpt += fmt.Sprintf(" DEFAULT \"%s\"", col.Default)
			}

			colLine = append(colLine, fmt.Sprintf("\t`%s` %s %s%s", col.Name, m.columnType(col), nullConstraint, columnOpt))
		}

		for _, i := range table.Indexes {
			idxName := i.Name
			if idxName == "" {
				idxName = "idx_" + strings.Join(i.Columns, "_")
			}
			cols := make([]string, 0, len(i.Columns))
			for _, c := range i.Columns {
				cols = append(cols, fmt.Sprintf("`%s`", c))
			}
			indexType := "INDEX"
			if i.Unique {
				indexType = "UNIQUE"
			}
			colLine = append(colLine, fmt.Sprintf("\t%s `%s` (%s)", indexType, idxName, strings.Join(cols, ",")))
		}

		if len(table.PrimaryKey) > 0 {
			primaryKeys := make([]string, 0, len(table.PrimaryKey))
			for _, p := range table.PrimaryKey {
				primaryKeys = append(primaryKeys, fmt.Sprintf("`%s`", p))
			}
			colLine = append(colLine, fmt.Sprintf("\tPRIMARY KEY(%s)", strings.Join(primaryKeys, ",")))
		}

		writer.WriteString(strings.Join(colLine, ",\n"))
		writer.WriteString("\n")
		engine := "InnoDB"
		if table.Engine != "" {
			engine = table.Engine
		}
		writer.WriteString(fmt.Sprintf(") Engine=%s;\n\n", engine))
	}

	writer.WriteString("SET foreign_key_checks=1;\n")
}

func (MySQLDialect) columnType(col Column) string {
	columnType := ""
	if t, ok := DataTypeMap[col.DataType]; ok {
		switch col.DataType {
		case descriptor.FieldDescriptorProto_TYPE_STRING:
			switch col.TypeName {
			case "text":
				columnType = "TEXT"
			default:
				columnType = t + "(" + strconv.Itoa(col.Size) + ")"
			}
		default:
			columnType = t
		}
		return columnType
	}
	if t, ok := ExtendTypeNameMap[col.TypeName]; ok {
		return t
	}

	return ""
}
