package migrate

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"errors"
	"fmt"
	"os"

	"log"
	"sync"
	"time"

	"github.com/go-sql-driver/mysql"
	"github.com/schemalex/schemalex"
	"go.f110.dev/xerrors"

	"go.f110.dev/protoc-ddl/internal/generator"
	"go.f110.dev/protoc-ddl/internal/schema"
)

const (
	sqliteDriver = "sqlite"
)

var (
	ErrMigrated = errors.New("already migrated")
)

var SchemaVersionTable *schema.Message

func init() {
	primaryKey := &schema.Field{Name: "sha256", Type: "TYPE_STRING", Size: 64}
	SchemaVersionTable = schema.NewMessage(nil, nil)
	SchemaVersionTable.TableName = "schema_version"
	SchemaVersionTable.Fields = schema.NewFields([]*schema.Field{
		primaryKey,
		{Name: "start_at", Type: schema.TimestampType},
		{Name: "finished_at", Type: schema.TimestampType, Null: true},
	})
	SchemaVersionTable.PrimaryKeys = []*schema.Field{primaryKey}
}

type Migration struct {
	driver             string
	schema             string
	versionTableSchema string
	schemaHash         string
	dsn                string

	db   *sql.DB
	mu   sync.Mutex
	lock *sql.Tx
	diff *mysqlDiff
}

// NewMigration returns the migration which applies schemaFile to the database. driver is mysql or sqlite.
func NewMigration(schemaFile, driver, dsn string) (*Migration, error) {
	if driver == sqliteDriver {
		sch, err := os.ReadFile(schemaFile)
		if err != nil {
			return nil, xerrors.WithStack(err)
		}
		db, err := sql.Open(sqliteDriver, dsn)
		if err != nil {
			return nil, xerrors.WithStack(err)
		}
		return NewSQLiteMigration(db, string(sch))
	}

	switch driver {
	case "mysql":
		cfg, err := mysql.ParseDSN(dsn)
		if err != nil {
			return nil, xerrors.WithStack(err)
		}
		cfg.MultiStatements = true
		cfg.ParseTime = true

		dsn = cfg.FormatDSN()
	}

	db, err := sql.Open(driver, dsn)
	if err != nil {
		return nil, xerrors.WithStack(err)
	}

	buf := new(bytes.Buffer)
	switch driver {
	case "mysql":
		generator.MySQLGenerator{}.Generate(buf, schema.NewMessages([]*schema.Message{SchemaVersionTable}, nil))
	}

	sch, err := os.ReadFile(schemaFile)
	if err != nil {
		return nil, xerrors.WithStack(err)
	}
	h := sha256.Sum256(sch)

	m := &Migration{driver: driver, schema: string(sch), versionTableSchema: buf.String(), schemaHash: hex.EncodeToString(h[:]), dsn: dsn, db: db}
	if err := m.plan(); err != nil {
		return nil, err
	}

	return m, nil
}

// NewSQLiteMigration returns the migration which applies ddl to db. db must be opened by modernc.org/sqlite.
// The migration is executed in a transaction started by BEGIN IMMEDIATE, so set busy_timeout to db if other processes
// may access the database at the same time.
func NewSQLiteMigration(db *sql.DB, ddl string) (*Migration, error) {
	buf := new(bytes.Buffer)
	if err := (generator.SQLiteGenerator{}).Generate(buf, schema.NewMessages([]*schema.Message{SchemaVersionTable}, nil)); err != nil {
		return nil, err
	}
	h := sha256.Sum256([]byte(ddl))

	return &Migration{driver: sqliteDriver, schema: ddl, versionTableSchema: buf.String(), schemaHash: hex.EncodeToString(h[:]), db: db}, nil
}

func (m *Migration) Execute(ctx context.Context, execute bool) error {
	if m.driver == sqliteDriver {
		return m.executeSQLite(ctx, execute)
	}

	if !execute {
		return m.dryRun()
	}

	if !m.checkSchemaVersionTable(ctx) {
		if err := m.createSchemaVersionTable(ctx); err != nil {
			return err
		}
	}

	if err := m.getLock(ctx); err != nil {
		if err == ErrMigrated {
			return nil
		}
		return err
	}

	for m.diff.Next() {
		log.Printf("Execte: %s", m.diff.Query())
		_, err := m.db.ExecContext(ctx, m.diff.Query())
		if err != nil {
			m.failure()
			return xerrors.WithStack(err)
		}
	}

	if err := m.finishMigration(ctx, time.Now()); err != nil {
		return err
	}
	return nil
}

func (m *Migration) dryRun() error {
	for m.diff.Next() {
		fmt.Println(m.diff.Query())
	}

	return nil
}

func (m *Migration) plan() error {
	targetDB := schemalex.NewMySQLSource(m.dsn)

	d, err := newMySQLDiff(targetDB, m.schema)
	if err != nil {
		return err
	}
	m.diff = d

	return nil
}

func (m *Migration) checkSchemaVersionTable(ctx context.Context) bool {
	row := m.db.QueryRowContext(ctx, fmt.Sprintf("SHOW CREATE TABLE %s", SchemaVersionTable.TableName))
	var table, sch string
	if err := row.Scan(&table, &sch); err != nil {
		return false
	}

	return true
}

func (m *Migration) createSchemaVersionTable(ctx context.Context) error {
	log.Printf("Create schema version table: %s", m.versionTableSchema)
	_, err := m.db.ExecContext(ctx, m.versionTableSchema)
	if err != nil {
		return xerrors.WithStack(err)
	}

	return nil
}

func (m *Migration) getLock(ctx context.Context) error {
	m.mu.Lock()
	if m.lock != nil {
		m.mu.Unlock()
		return xerrors.New("migrate: Already locked by other instance")
	}
	m.mu.Unlock()

	_, err := m.db.ExecContext(ctx, fmt.Sprintf("INSERT INTO `%s` (`sha256`, `start_at`) VALUES (?, ?)", SchemaVersionTable.TableName), m.schemaHash, time.Now())
	if err != nil {
		mysqlErr, ok := err.(*mysql.MySQLError)
		if !ok {
			return xerrors.WithStack(err)
		}
		if mysqlErr.Number != 1062 {
			return xerrors.WithStack(err)
		}
	}

	tx, err := m.db.BeginTx(ctx, &sql.TxOptions{})
	if err != nil {
		return xerrors.WithStack(err)
	}
	row := tx.QueryRowContext(ctx, fmt.Sprintf("SELECT `finished_at` FROM `%s` WHERE `sha256` = ? FOR UPDATE", SchemaVersionTable.TableName), m.schemaHash)
	var finishedAt *time.Time
	if err := row.Scan(&finishedAt); err != nil {
		tx.Rollback()
		return xerrors.WithStack(err)
	}
	if finishedAt != nil && !finishedAt.IsZero() {
		tx.Rollback()
		return ErrMigrated
	}

	m.mu.Lock()
	m.lock = tx
	m.mu.Unlock()

	return nil
}

func (m *Migration) failure() error {
	m.mu.Lock()
	defer m.mu.Unlock()

	m.lock.Rollback()
	m.lock = nil
	return nil
}

func (m *Migration) finishMigration(ctx context.Context, now time.Time) error {
	m.mu.Lock()
	defer m.mu.Unlock()

	result, err := m.lock.ExecContext(ctx, fmt.Sprintf("UPDATE `%s` SET `finished_at` = ? WHERE `sha256` = ?", SchemaVersionTable.TableName), now, m.schemaHash)
	if err != nil {
		m.lock.Rollback()
		m.lock = nil
		return xerrors.WithStack(err)
	}
	n, err := result.RowsAffected()
	if err != nil {
		m.lock.Rollback()
		m.lock = nil
		return xerrors.WithStack(err)
	}
	if n != 1 {
		m.lock.Rollback()
		m.lock = nil
		return xerrors.WithMessagef(err, "migrate: Failed update schema version table")
	}

	if err := m.lock.Commit(); err != nil {
		m.lock = nil
		return xerrors.WithStack(err)
	}

	m.lock = nil
	return nil
}

func (m *Migration) executeSQLite(ctx context.Context, execute bool) error {
	conn, err := m.db.Conn(ctx)
	if err != nil {
		return xerrors.WithStack(err)
	}
	defer conn.Close()

	if !execute {
		queries, err := sqliteDiff(ctx, conn, m.schema)
		if err != nil {
			return err
		}
		for _, q := range queries {
			fmt.Println(q)
		}
		return nil
	}

	if _, err := conn.ExecContext(ctx, "BEGIN IMMEDIATE"); err != nil {
		return xerrors.WithStack(err)
	}
	if err := m.migrateSQLite(ctx, conn); err != nil {
		conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		return err
	}
	if _, err := conn.ExecContext(ctx, "COMMIT"); err != nil {
		conn.ExecContext(context.WithoutCancel(ctx), "ROLLBACK")
		return xerrors.WithStack(err)
	}

	return nil
}

func (m *Migration) migrateSQLite(ctx context.Context, conn *sql.Conn) error {
	startAt := time.Now()

	var n int
	row := conn.QueryRowContext(ctx, "SELECT COUNT(*) FROM `sqlite_master` WHERE `type` = 'table' AND `name` = ?", SchemaVersionTable.TableName)
	if err := row.Scan(&n); err != nil {
		return xerrors.WithStack(err)
	}
	if n == 0 {
		log.Printf("Create schema version table: %s", m.versionTableSchema)
		if _, err := conn.ExecContext(ctx, m.versionTableSchema); err != nil {
			return xerrors.WithStack(err)
		}
	}

	var finishedAt *time.Time
	row = conn.QueryRowContext(ctx, fmt.Sprintf("SELECT `finished_at` FROM `%s` WHERE `sha256` = ?", SchemaVersionTable.TableName), m.schemaHash)
	if err := row.Scan(&finishedAt); err != nil && !errors.Is(err, sql.ErrNoRows) {
		return xerrors.WithStack(err)
	}
	if finishedAt != nil {
		return nil
	}

	queries, err := sqliteDiff(ctx, conn, m.schema)
	if err != nil {
		return err
	}
	for _, q := range queries {
		log.Printf("Execute: %s", q)
		if _, err := conn.ExecContext(ctx, q); err != nil {
			return xerrors.WithStack(err)
		}
	}

	_, err = conn.ExecContext(
		ctx,
		fmt.Sprintf("INSERT OR REPLACE INTO `%s` (`sha256`, `start_at`, `finished_at`) VALUES (?, ?, ?)", SchemaVersionTable.TableName),
		m.schemaHash, startAt, time.Now(),
	)
	if err != nil {
		return xerrors.WithStack(err)
	}

	return nil
}
