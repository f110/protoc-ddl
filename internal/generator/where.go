package generator

import (
	"fmt"
	"strconv"
	"strings"

	"github.com/pingcap/tidb/pkg/parser/ast"
	"github.com/pingcap/tidb/pkg/parser/opcode"
	"github.com/pingcap/tidb/pkg/parser/test_driver"
	"github.com/pingcap/tidb/pkg/parser/types"

	"go.f110.dev/protoc-ddl/internal/schema"
)

var comparisonOperators = map[opcode.Op]string{
	opcode.EQ: "=",
	opcode.NE: "!=",
	opcode.LT: "<",
	opcode.LE: "<=",
	opcode.GT: ">",
	opcode.GE: ">=",
}

// queryArg is an argument of the query. If slice is true, the argument is a slice for IN.
type queryArg struct {
	name  string
	field *schema.Field
	slice bool
}

func (a *queryArg) goType() string {
	if a.slice {
		return "[]" + GoDataTypeMap[a.field.Type]
	}
	return GoDataTypeMap[a.field.Type]
}

// whereNode is a node of the condition tree of WHERE clause. A branch node has op and children. A leaf node has sql
// and optionally arg or value. value is the Go literal which is given to the placeholder.
type whereNode struct {
	op       string
	children []*whereNode
	sql      string
	arg      *queryArg
	value    string
}

// needsConditionTree reports whether the expression has LIKE, IN or parentheses. These expressions are generated with
// the condition tree.
func needsConditionTree(e ast.ExprNode) bool {
	switch v := e.(type) {
	case *ast.PatternInExpr, *ast.PatternLikeOrIlikeExpr, *ast.ParenthesesExpr:
		return true
	case *ast.BinaryOperationExpr:
		return needsConditionTree(v.L) || needsConditionTree(v.R)
	}
	return false
}

func newWhereNode(m *schema.Message, e ast.ExprNode) (*whereNode, error) {
	fields := make(map[string]*schema.Field)
	for _, v := range m.Fields.List() {
		fields[v.Name] = v
	}
	b := &whereBuilder{fields: fields, names: make(map[string]int)}
	return b.build(e)
}

type whereBuilder struct {
	fields map[string]*schema.Field
	names  map[string]int
}

func (b *whereBuilder) build(e ast.ExprNode) (*whereNode, error) {
	switch v := e.(type) {
	case *ast.ParenthesesExpr:
		return b.build(v.Expr)
	case *ast.BinaryOperationExpr:
		switch v.Op {
		case opcode.LogicAnd, opcode.LogicOr:
			op := "AND"
			if v.Op == opcode.LogicOr {
				op = "OR"
			}
			n := &whereNode{op: op}
			for _, c := range []ast.ExprNode{v.L, v.R} {
				child, err := b.build(c)
				if err != nil {
					return nil, err
				}
				if child.op == op {
					n.children = append(n.children, child.children...)
				} else {
					n.children = append(n.children, child)
				}
			}
			return n, nil
		}
		op, ok := comparisonOperators[v.Op]
		if !ok {
			return nil, fmt.Errorf("operator %s is not supported", v.Op)
		}
		col, err := b.column(v.L)
		if err != nil {
			return nil, err
		}
		switch r := v.R.(type) {
		case *test_driver.ParamMarkerExpr:
			arg, err := b.arg(col, false)
			if err != nil {
				return nil, err
			}
			return &whereNode{sql: fmt.Sprintf("`%s` %s ?", col, op), arg: arg}, nil
		case *test_driver.ValueExpr:
			switch r.Type.EvalType() {
			case types.ETInt:
				return &whereNode{sql: fmt.Sprintf("`%s` %s %d", col, op, r.GetInt64())}, nil
			case types.ETString:
				return &whereNode{sql: fmt.Sprintf("`%s` %s ?", col, op), value: strconv.Quote(r.GetString())}, nil
			}
			return nil, fmt.Errorf("the literal of %s is not supported", r.Type.EvalType())
		}
		return nil, fmt.Errorf("%T is not supported on the right side of %s", v.R, op)
	case *ast.PatternLikeOrIlikeExpr:
		col, err := b.column(v.Expr)
		if err != nil {
			return nil, err
		}
		if !v.IsLike {
			return nil, fmt.Errorf("ILIKE is not supported")
		}
		if _, ok := v.Pattern.(*test_driver.ParamMarkerExpr); !ok {
			return nil, fmt.Errorf("only the placeholder is supported as the pattern of LIKE")
		}
		arg, err := b.arg(col, false)
		if err != nil {
			return nil, err
		}
		op := "LIKE"
		if v.Not {
			op = "NOT LIKE"
		}
		return &whereNode{sql: fmt.Sprintf("`%s` %s ?", col, op), arg: arg}, nil
	case *ast.PatternInExpr:
		col, err := b.column(v.Expr)
		if err != nil {
			return nil, err
		}
		if v.Not || v.Sel != nil || len(v.List) != 1 {
			return nil, fmt.Errorf("only IN (?) is supported")
		}
		if _, ok := v.List[0].(*test_driver.ParamMarkerExpr); !ok {
			return nil, fmt.Errorf("only IN (?) is supported")
		}
		arg, err := b.arg(col, true)
		if err != nil {
			return nil, err
		}
		return &whereNode{sql: fmt.Sprintf("`%s`", col), arg: arg}, nil
	case *ast.IsNullExpr:
		col, err := b.column(v.Expr)
		if err != nil {
			return nil, err
		}
		if v.Not {
			return &whereNode{sql: fmt.Sprintf("`%s` IS NOT NULL", col)}, nil
		}
		return &whereNode{sql: fmt.Sprintf("`%s` IS NULL", col)}, nil
	}
	return nil, fmt.Errorf("%T is not supported", e)
}

func (b *whereBuilder) column(e ast.ExprNode) (string, error) {
	c, ok := e.(*ast.ColumnNameExpr)
	if !ok {
		return "", fmt.Errorf("%T is not supported as the column", e)
	}
	return c.Name.Name.String(), nil
}

func (b *whereBuilder) arg(col string, slice bool) (*queryArg, error) {
	f, ok := b.fields[col]
	if !ok {
		return nil, fmt.Errorf("column %s is not found", col)
	}
	name := schema.ToLowerCamel(f.Name)
	b.names[name]++
	if n := b.names[name]; n > 1 {
		name += strconv.Itoa(n)
	}
	return &queryArg{name: name, field: f, slice: slice}, nil
}

// args returns the arguments in the order of the placeholders.
func (n *whereNode) args() []*queryArg {
	if n.op == "" {
		if n.arg == nil {
			return nil
		}
		return []*queryArg{n.arg}
	}
	var res []*queryArg
	for _, c := range n.children {
		res = append(res, c.args()...)
	}
	return res
}

// goExpr returns the Go expression which builds the condition with the functions of whereConditionSource.
func (n *whereNode) goExpr() string {
	switch n.op {
	case "AND", "OR":
		children := make([]string, len(n.children))
		for i, c := range n.children {
			children[i] = c.goExpr()
		}
		fn := "whereAnd"
		if n.op == "OR" {
			fn = "whereOr"
		}
		return fmt.Sprintf("%s(%s)", fn, strings.Join(children, ", "))
	}
	if n.value != "" {
		return fmt.Sprintf("whereCond(%q, %s)", n.sql, n.value)
	}
	if n.arg == nil {
		return fmt.Sprintf("whereCond(%q)", n.sql)
	}
	if n.arg.slice {
		return fmt.Sprintf("whereIn(%q, %s)", n.sql, n.arg.name)
	}
	return fmt.Sprintf("whereCond(%q, %s)", n.sql, n.arg.name)
}

// whereConditionSource is the source code which builds WHERE clause at runtime. The functions are unexported so that
// a raw SQL can not be given from outside of the generated package.
const whereConditionSource = `
type whereCondition struct {
	sql  string
	args []any
}

func whereCond(sql string, args ...any) *whereCondition {
	return &whereCondition{sql: sql, args: args}
}

// whereIn returns the condition that column is one of values. If values is empty, the condition is always false.
func whereIn[T any](column string, values []T) *whereCondition {
	if len(values) == 0 {
		return &whereCondition{sql: "1 = 0"}
	}
	args := make([]any, len(values))
	for i, v := range values {
		args[i] = v
	}
	return &whereCondition{sql: column + " IN (" + strings.TrimSuffix(strings.Repeat("?, ", len(values)), ", ") + ")", args: args}
}

func whereAnd(conds ...*whereCondition) *whereCondition {
	return whereJoin("AND", conds)
}

func whereOr(conds ...*whereCondition) *whereCondition {
	return whereJoin("OR", conds)
}

func whereJoin(op string, conds []*whereCondition) *whereCondition {
	sqls := make([]string, len(conds))
	var args []any
	for i, c := range conds {
		sqls[i] = c.sql
		args = append(args, c.args...)
	}
	return &whereCondition{sql: "(" + strings.Join(sqls, " "+op+" ") + ")", args: args}
}
`
