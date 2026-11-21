package migrate

import (
	"bytes"
	"context"
	"database/sql"
	"fmt"
	"maps"
	"reflect"
	"slices"
	"sort"
	"strings"

	mapset "github.com/deckarep/golang-set/v2"
	"github.com/schemalex/schemalex"
	"github.com/schemalex/schemalex/format"
	"github.com/schemalex/schemalex/model"
	"go.f110.dev/xerrors"
	_ "modernc.org/sqlite"
)

type mysqlDiff struct {
	fromStatement model.Stmts
	toStatement   model.Stmts

	index   int
	queries []string
}

func newMySQLDiff(from schemalex.SchemaSource, to string) (*mysqlDiff, error) {
	buf := new(bytes.Buffer)
	if err := from.WriteSchema(buf); err != nil {
		return nil, xerrors.WithStack(err)
	}
	parser := schemalex.New()
	fromStatement, err := parser.Parse(buf.Bytes())
	if err != nil {
		return nil, xerrors.WithStack(err)
	}

	toStatement, err := parser.ParseString(to)
	if err != nil {
		return nil, err
	}

	d := &mysqlDiff{fromStatement: fromStatement, toStatement: toStatement, index: -1}
	if err := d.parse(); err != nil {
		return nil, err
	}

	return d, nil
}

func (d *mysqlDiff) Next() bool {
	if d.index+1 < len(d.queries) {
		d.index++
		return true
	}

	return false
}

func (d *mysqlDiff) Query() string {
	return d.queries[d.index]
}

func (d *mysqlDiff) parse() error {
	fromSet := mapset.NewSet[string]()
	for _, stmt := range d.fromStatement {
		if cs, ok := stmt.(model.Table); ok {
			if cs.Name() == SchemaVersionTable.TableName {
				continue
			}
			fromSet.Add(cs.ID())
		}
	}
	toSet := mapset.NewSet[string]()
	for _, stmt := range d.toStatement {
		if cs, ok := stmt.(model.Table); ok {
			toSet.Add(cs.ID())
		}
	}

	if err := d.dropTable(fromSet, toSet); err != nil {
		return err
	}
	if err := d.createTable(fromSet, toSet); err != nil {
		return err
	}
	if err := d.alterTable(fromSet, toSet); err != nil {
		return err
	}

	return nil
}

func (d *mysqlDiff) addQuery(query string) {
	d.queries = append(d.queries, query)
}

func (d *mysqlDiff) dropTable(fromSet, toSet mapset.Set[string]) error {
	var buf bytes.Buffer
	ids := fromSet.Difference(toSet)
	for i, id := range ids.ToSlice() {
		if i > 0 {
			buf.WriteByte('\n')
		}

		stmt, ok := d.fromStatement.Lookup(id)
		if !ok {
			return xerrors.WithStack(xerrors.Newf("%s not found from source", id))
		}

		table, ok := stmt.(model.Table)
		if !ok {
			return xerrors.WithStack(xerrors.Newf("%s not found from target", id))
		}
		d.addQuery(fmt.Sprintf("DROP TABLE `%s`", table.Name()))
	}

	return nil
}

func (d *mysqlDiff) createTable(fromSet, toSet mapset.Set[string]) error {
	ids := toSet.Difference(fromSet)
	buf := new(bytes.Buffer)
	for _, id := range ids.ToSlice() {
		stmt, ok := d.toStatement.Lookup(id)
		if !ok {
			return xerrors.WithStack(xerrors.Newf("%s not found from target", id))
		}

		if err := format.SQL(buf, stmt); err != nil {
			return xerrors.WithStack(err)
		}
		d.addQuery(buf.String())
		buf.Reset()
	}

	return nil
}

func (d *mysqlDiff) alterTable(fromSet, toSet mapset.Set[string]) error {
	ids := toSet.Intersect(fromSet)
	for _, id := range ids.ToSlice() {
		var stmt model.Stmt
		var ok bool

		stmt, ok = d.fromStatement.Lookup(id)
		if !ok {
			return xerrors.WithStack(xerrors.Newf("%s not found from source", id))
		}
		beforeStmt := stmt.(model.Table)

		stmt, ok = d.toStatement.Lookup(id)
		if !ok {
			return xerrors.WithStack(xerrors.Newf("%s not found from target", id))
		}
		afterStmt := stmt.(model.Table)

		fromColumns := mapset.NewSet[string]()
		for col := range beforeStmt.Columns() {
			fromColumns.Add(col.ID())
		}

		toColumns := mapset.NewSet[string]()
		for col := range afterStmt.Columns() {
			toColumns.Add(col.ID())
		}

		fromIndexes := mapset.NewSet[string]()
		for idx := range beforeStmt.Indexes() {
			fromIndexes.Add(idx.ID())
		}

		toIndexes := mapset.NewSet[string]()
		for idx := range afterStmt.Indexes() {
			toIndexes.Add(idx.ID())
		}

		if err := d.dropTableIndexes(beforeStmt, afterStmt, fromIndexes, toIndexes); err != nil {
			return err
		}
		if err := d.dropTableColumns(beforeStmt, fromColumns, toColumns); err != nil {
			return err
		}
		if err := d.addTableColumns(beforeStmt, afterStmt, fromColumns, toColumns); err != nil {
			return err
		}
		if err := d.alterTableColumns(beforeStmt, afterStmt, fromColumns, toColumns); err != nil {
			return err
		}
		if err := d.addTableIndexes(beforeStmt, afterStmt, fromIndexes, toIndexes); err != nil {
			return err
		}
	}

	return nil
}

func (d *mysqlDiff) dropTableIndexes(before, after model.Table, fromIndexes, toIndexes mapset.Set[string]) error {
	indexes := fromIndexes.Difference(toIndexes)
	lazy := make([]model.Index, 0, indexes.Cardinality())
	for _, index := range indexes.ToSlice() {
		indexStmt, ok := before.LookupIndex(index)
		if !ok {
			return xerrors.WithStack(xerrors.Newf("%s not found from source", index))
		}

		if indexStmt.IsPrimaryKey() {
			d.addQuery(fmt.Sprintf("ALTER TABLE `%s` DROP PRIMARY KEY", before.Name()))
			continue
		}

		if !indexStmt.HasName() && !indexStmt.HasSymbol() {
			return xerrors.WithStack(xerrors.Newf("can not drop index without name: %s", indexStmt.ID()))
		}
		if !indexStmt.IsForeignKey() {
			lazy = append(lazy, indexStmt)
			continue
		}

		var name string
		if indexStmt.HasSymbol() {
			name = indexStmt.Symbol()
		} else {
			name = indexStmt.Name()
		}
		d.addQuery(fmt.Sprintf("ALTER TABLE `%s` DROP FOREIGN KEY `%s`", before.Name(), name))
	}

	for _, indexStmt := range lazy {
		var name string
		if !indexStmt.HasName() {
			name = indexStmt.Symbol()
		} else {
			name = indexStmt.Name()
		}
		d.addQuery(fmt.Sprintf("ALTER TABLE `%s` DROP INDEX `%s`", before.Name(), name))
	}

	return nil
}

func (d *mysqlDiff) dropTableColumns(before model.Table, fromColumns, toColumns mapset.Set[string]) error {
	columnNames := fromColumns.Difference(toColumns)

	for _, columnName := range columnNames.ToSlice() {
		col, ok := before.LookupColumn(columnName)
		if !ok {
			return xerrors.WithStack(xerrors.Newf(`failed to lookup column %s`, columnName))
		}

		d.addQuery(fmt.Sprintf("ALTER TABLE `%s` DROP COLUMN `%s`", before.Name(), col.Name()))
	}

	return nil
}

func (d *mysqlDiff) addTableColumns(before, after model.Table, fromColumns, toColumns mapset.Set[string]) error {
	beforeToNext := make(map[string]string)
	nextToBefore := make(map[string]string)

	var firstColumn model.TableColumn
	for _, v := range toColumns.Difference(fromColumns).ToSlice() {
		columnName := v
		col, ok := after.LookupColumn(columnName)
		if !ok {
			return xerrors.WithStack(xerrors.Newf(`failed to lookup column %s`, columnName))
		}

		beforeCol, hasBeforeCol := after.LookupColumnBefore(col.ID())
		if !hasBeforeCol {
			firstColumn = col
			continue
		}

		beforeToNext[beforeCol.ID()] = columnName
		nextToBefore[columnName] = beforeCol.ID()
	}

	if firstColumn != nil {
		if err := d.writeAddColumnQuery(before, after, firstColumn.ID()); err != nil {
			return err
		}
	}

	var columnNames []string
	for _, v := range toColumns.Intersect(fromColumns).ToSlice() {
		columnName := v
		if nextColumnName, ok := beforeToNext[columnName]; ok {
			delete(beforeToNext, columnName)
			delete(nextToBefore, nextColumnName)
			columnNames = append(columnNames, nextColumnName)
		}
	}

	if len(columnNames) > 0 {
		sort.Strings(columnNames)
		if err := d.writeAddColumnQuery(before, after, columnNames...); err != nil {
			return err
		}
	}

	columnNames = columnNames[:0]
	for _, nextCol := range beforeToNext {
		columnNames = append(columnNames, nextCol)
	}
	if len(columnNames) > 0 {
		sort.Slice(columnNames, func(i, j int) bool {
			icol, _ := after.LookupColumnOrder(columnNames[i])
			jcol, _ := after.LookupColumnOrder(columnNames[j])
			return icol < jcol
		})
		if err := d.writeAddColumnQuery(before, after, columnNames...); err != nil {
			return err
		}
	}

	return nil
}

func (d *mysqlDiff) writeAddColumnQuery(before, after model.Table, columnNames ...string) error {
	buf := new(bytes.Buffer)
	for _, columnName := range columnNames {
		stmt, ok := after.LookupColumn(columnName)
		if !ok {
			return xerrors.WithStack(xerrors.Newf(`failed to lookup column %s`, columnName))
		}

		beforeCol, hasBeforeCol := after.LookupColumnBefore(stmt.ID())
		buf.WriteString("ALTER TABLE `")
		buf.WriteString(before.Name())
		buf.WriteString("` ADD COLUMN ")
		if err := format.SQL(buf, stmt); err != nil {
			return xerrors.WithStack(err)
		}

		if hasBeforeCol {
			buf.WriteString(" AFTER `")
			buf.WriteString(beforeCol.Name())
			buf.WriteString("`")
		} else {
			buf.WriteString(" FIRST")
		}

		d.addQuery(buf.String())
		buf.Reset()
	}
	return nil
}

func (d *mysqlDiff) alterTableColumns(before, after model.Table, fromColumns, toColumns mapset.Set[string]) error {
	buf := new(bytes.Buffer)
	columnNames := toColumns.Intersect(fromColumns)
	for _, columnName := range columnNames.ToSlice() {
		beforeColumnStmt, ok := before.LookupColumn(columnName)
		if !ok {
			return xerrors.WithStack(xerrors.Newf(`column %s not found in old schema`, columnName))
		}

		afterColumnStmt, ok := after.LookupColumn(columnName)
		if !ok {
			return xerrors.WithStack(xerrors.Newf(`column %s not found in new schema`, columnName))
		}

		if reflect.DeepEqual(beforeColumnStmt, afterColumnStmt) {
			continue
		}
		if err := format.SQL(buf, afterColumnStmt); err != nil {
			return xerrors.WithStack(err)
		}

		d.addQuery(fmt.Sprintf("ALTER TABLE `%s` CHANGE COLUMN `%s` %s", before.Name(), afterColumnStmt.Name(), buf.String()))
		buf.Reset()
	}

	return nil
}

func (d *mysqlDiff) addTableIndexes(before, after model.Table, fromIndexes, toIndexes mapset.Set[string]) error {
	buf := new(bytes.Buffer)
	indexes := toIndexes.Difference(fromIndexes)
	lazy := make([]model.Index, 0, indexes.Cardinality())
	for _, index := range indexes.ToSlice() {
		indexStmt, ok := after.LookupIndex(index)
		if !ok {
			return xerrors.WithStack(xerrors.Newf(`index '%s' not found in old schema (add index)`, index))
		}
		if indexStmt.IsForeignKey() {
			lazy = append(lazy, indexStmt)
			continue
		}

		if err := format.SQL(buf, indexStmt); err != nil {
			return xerrors.WithStack(err)
		}

		d.addQuery(fmt.Sprintf("ALTER TABLE `%s` ADD %s", before.Name(), buf.String()))
		buf.Reset()
	}

	for _, indexStmt := range lazy {
		if err := format.SQL(buf, indexStmt); err != nil {
			return xerrors.WithStack(err)
		}

		d.addQuery(fmt.Sprintf("ALTER TABGLE `%s` ADD %s", before.Name(), buf.String()))
		buf.Reset()
	}

	return nil
}

type sqliteQuerier interface {
	QueryContext(ctx context.Context, query string, args ...any) (*sql.Rows, error)
}

type sqliteTable struct {
	name          string
	sql           string
	columns       []sqliteColumn
	indexes       map[string]sqliteIndex
	autoIncrement bool
}

func (t *sqliteTable) sameDefinition(other *sqliteTable) bool {
	return t.autoIncrement == other.autoIncrement && slices.EqualFunc(t.columns, other.columns, func(a, b sqliteColumn) bool {
		return a.name == b.name && strings.EqualFold(a.typ, b.typ) && a.notNull == b.notNull && a.defaultValue == b.defaultValue &&
			a.primaryKey == b.primaryKey
	})
}

func (t *sqliteTable) column(name string) (sqliteColumn, bool) {
	for _, v := range t.columns {
		if v.name == name {
			return v, true
		}
	}
	return sqliteColumn{}, false
}

func (t *sqliteTable) indexNames() []string {
	return slices.Sorted(maps.Keys(t.indexes))
}

type sqliteColumn struct {
	name         string
	typ          string
	notNull      bool
	defaultValue sql.NullString
	primaryKey   int
}

type sqliteIndex struct {
	name    string
	unique  bool
	columns []string
	sql     string
}

func (i sqliteIndex) sameDefinition(other sqliteIndex) bool {
	return i.unique == other.unique && slices.Equal(i.columns, other.columns)
}

// sqliteDiff returns the queries which change the schema of current to ddl.
// The table which has the different column definitions is recreated and the data is copied from the old table.
func sqliteDiff(ctx context.Context, current sqliteQuerier, ddl string) ([]string, error) {
	desiredDB, err := sql.Open(sqliteDriver, ":memory:")
	if err != nil {
		return nil, xerrors.WithStack(err)
	}
	defer desiredDB.Close()
	desiredDB.SetMaxOpenConns(1)
	if _, err := desiredDB.ExecContext(ctx, ddl); err != nil {
		return nil, xerrors.WithStack(err)
	}

	from, err := inspectSQLite(ctx, current)
	if err != nil {
		return nil, err
	}
	to, err := inspectSQLite(ctx, desiredDB)
	if err != nil {
		return nil, err
	}

	var drops, tables, indexes []string
	for _, name := range slices.Sorted(maps.Keys(from)) {
		if _, ok := to[name]; !ok {
			drops = append(drops, fmt.Sprintf("DROP TABLE `%s`", name))
		}
	}
	for _, name := range slices.Sorted(maps.Keys(to)) {
		desired := to[name]
		currentTable, ok := from[name]
		if !ok {
			tables = append(tables, desired.sql)
			for _, idx := range desired.indexNames() {
				indexes = append(indexes, desired.indexes[idx].sql)
			}
			continue
		}

		if !currentTable.sameDefinition(desired) {
			for _, idx := range currentTable.indexNames() {
				drops = append(drops, fmt.Sprintf("DROP INDEX `%s`", idx))
			}
			tables = append(tables, recreateSQLiteTable(currentTable, desired)...)
			for _, idx := range desired.indexNames() {
				indexes = append(indexes, desired.indexes[idx].sql)
			}
			continue
		}

		for _, idx := range currentTable.indexNames() {
			if v, ok := desired.indexes[idx]; !ok || !v.sameDefinition(currentTable.indexes[idx]) {
				drops = append(drops, fmt.Sprintf("DROP INDEX `%s`", idx))
			}
		}
		for _, idx := range desired.indexNames() {
			if v, ok := currentTable.indexes[idx]; !ok || !v.sameDefinition(desired.indexes[idx]) {
				indexes = append(indexes, desired.indexes[idx].sql)
			}
		}
	}

	return slices.Concat(drops, tables, indexes), nil
}

func recreateSQLiteTable(current, desired *sqliteTable) []string {
	oldTable := "_protoc_ddl_old_" + current.name
	queries := []string{
		fmt.Sprintf("ALTER TABLE `%s` RENAME TO `%s`", current.name, oldTable),
		desired.sql,
	}

	var columns, values []string
	for _, col := range desired.columns {
		if _, ok := current.column(col.name); ok {
			columns = append(columns, fmt.Sprintf("`%s`", col.name))
			values = append(values, fmt.Sprintf("`%s`", col.name))
			continue
		}
		if col.notNull && !col.defaultValue.Valid && col.primaryKey == 0 {
			columns = append(columns, fmt.Sprintf("`%s`", col.name))
			values = append(values, sqliteZeroValue(col.typ))
		}
	}
	if len(columns) > 0 {
		queries = append(queries, fmt.Sprintf("INSERT INTO `%s` (%s) SELECT %s FROM `%s`",
			desired.name, strings.Join(columns, ", "), strings.Join(values, ", "), oldTable))
	}

	return append(queries, fmt.Sprintf("DROP TABLE `%s`", oldTable))
}

// sqliteZeroValue returns the value for the new NOT NULL column without the default value like MySQL.
// The type is classified by the rules of the type affinity.
func sqliteZeroValue(typ string) string {
	typ = strings.ToUpper(typ)
	switch {
	case strings.Contains(typ, "INT"):
		return "0"
	case strings.Contains(typ, "DATE"), strings.Contains(typ, "TIME"):
		return "'0001-01-01 00:00:00+00:00'"
	case strings.Contains(typ, "CHAR"), strings.Contains(typ, "CLOB"), strings.Contains(typ, "TEXT"):
		return "''"
	case strings.Contains(typ, "BLOB"):
		return "X''"
	default:
		return "0"
	}
}

func inspectSQLite(ctx context.Context, q sqliteQuerier) (map[string]*sqliteTable, error) {
	tables := make(map[string]*sqliteTable)
	err := querySQLite(ctx, q, func(rows *sql.Rows) error {
		t := &sqliteTable{indexes: make(map[string]sqliteIndex)}
		if err := rows.Scan(&t.name, &t.sql); err != nil {
			return err
		}
		t.autoIncrement = strings.Contains(strings.ToUpper(t.sql), "AUTOINCREMENT")
		tables[t.name] = t
		return nil
	}, "SELECT `name`, `sql` FROM `sqlite_master` WHERE `type` = 'table' AND `name` NOT LIKE 'sqlite\\_%' ESCAPE '\\' AND `name` != ?",
		SchemaVersionTable.TableName)
	if err != nil {
		return nil, err
	}

	for _, t := range tables {
		err := querySQLite(ctx, q, func(rows *sql.Rows) error {
			var col sqliteColumn
			if err := rows.Scan(&col.name, &col.typ, &col.notNull, &col.defaultValue, &col.primaryKey); err != nil {
				return err
			}
			t.columns = append(t.columns, col)
			return nil
		}, "SELECT `name`, `type`, `notnull`, `dflt_value`, `pk` FROM pragma_table_info(?) ORDER BY `cid`", t.name)
		if err != nil {
			return nil, err
		}

		err = querySQLite(ctx, q, func(rows *sql.Rows) error {
			var idx sqliteIndex
			if err := rows.Scan(&idx.name, &idx.unique, &idx.sql); err != nil {
				return err
			}
			t.indexes[idx.name] = idx
			return nil
		}, "SELECT `il`.`name`, `il`.`unique`, `m`.`sql` FROM pragma_index_list(?) AS `il` "+
			"INNER JOIN `sqlite_master` AS `m` ON `m`.`type` = 'index' AND `m`.`name` = `il`.`name` WHERE `il`.`origin` = 'c'",
			t.name)
		if err != nil {
			return nil, err
		}

		for name, idx := range t.indexes {
			err := querySQLite(ctx, q, func(rows *sql.Rows) error {
				var col string
				if err := rows.Scan(&col); err != nil {
					return err
				}
				idx.columns = append(idx.columns, col)
				return nil
			}, "SELECT `name` FROM pragma_index_info(?) ORDER BY `seqno`", name)
			if err != nil {
				return nil, err
			}
			t.indexes[name] = idx
		}
	}

	return tables, nil
}

func querySQLite(ctx context.Context, q sqliteQuerier, scan func(rows *sql.Rows) error, query string, args ...any) error {
	rows, err := q.QueryContext(ctx, query, args...)
	if err != nil {
		return xerrors.WithStack(err)
	}
	defer rows.Close()

	for rows.Next() {
		if err := scan(rows); err != nil {
			return xerrors.WithStack(err)
		}
	}
	return xerrors.WithStack(rows.Err())
}
