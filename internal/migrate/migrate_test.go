package migrate

import (
	"context"
	"database/sql"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const (
	sqliteSchemaV1 = "DROP TABLE IF EXISTS `user`;\n" +
		"CREATE TABLE `user` (\n" +
		"\t`id` INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,\n" +
		"\t`name` TEXT NOT NULL,\n" +
		"\t`age` INTEGER NULL,\n" +
		"\t`legacy` TEXT NOT NULL DEFAULT ''\n" +
		");\n" +
		"CREATE INDEX `idx_name` ON `user` (`name`);\n" +
		"CREATE INDEX `idx_age` ON `user` (`age`);\n" +
		"DROP TABLE IF EXISTS `obsolete`;\n" +
		"CREATE TABLE `obsolete` (\n" +
		"\t`id` INTEGER NOT NULL,\n" +
		"\tPRIMARY KEY(`id`)\n" +
		");\n" +
		"DROP TABLE IF EXISTS `tag`;\n" +
		"CREATE TABLE `tag` (\n" +
		"\t`id` INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,\n" +
		"\t`name` TEXT NOT NULL\n" +
		");\n" +
		"CREATE INDEX `idx_tag_name` ON `tag` (`name`);\n"
	sqliteSchemaV2 = "DROP TABLE IF EXISTS `user`;\n" +
		"CREATE TABLE `user` (\n" +
		"\t`id` INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,\n" +
		"\t`name` TEXT NOT NULL,\n" +
		"\t`age` TEXT NULL,\n" +
		"\t`email` TEXT NOT NULL,\n" +
		"\t`score` REAL NOT NULL DEFAULT '1.5',\n" +
		"\t`created_at` DATETIME NOT NULL\n" +
		");\n" +
		"CREATE UNIQUE INDEX `idx_name` ON `user` (`name`);\n" +
		"CREATE INDEX `idx_age` ON `user` (`age`);\n" +
		"DROP TABLE IF EXISTS `tag`;\n" +
		"CREATE TABLE `tag` (\n" +
		"\t`id` INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,\n" +
		"\t`name` TEXT NOT NULL\n" +
		");\n" +
		"CREATE UNIQUE INDEX `idx_tag_name` ON `tag` (`name`);\n" +
		"DROP TABLE IF EXISTS `blog`;\n" +
		"CREATE TABLE `blog` (\n" +
		"\t`id` INTEGER NOT NULL PRIMARY KEY AUTOINCREMENT,\n" +
		"\t`title` TEXT NOT NULL\n" +
		");\n" +
		"CREATE INDEX `idx_title` ON `blog` (`title`);\n"
)

func TestSQLiteMigration(t *testing.T) {
	ctx := context.Background()

	t.Run("Create", func(t *testing.T) {
		db := openSQLite(t)

		m, err := NewSQLiteMigration(db, sqliteSchemaV1)
		require.NoError(t, err)
		require.NoError(t, m.Execute(ctx, true))

		assert.ElementsMatch(t, []string{"user", "obsolete", "tag", "schema_version"}, sqliteObjects(t, db, "table"))
		assert.ElementsMatch(t, []string{"idx_name", "idx_age", "idx_tag_name"}, sqliteObjects(t, db, "index"))
		var finishedAt *time.Time
		require.NoError(t, db.QueryRow("SELECT `finished_at` FROM `schema_version` WHERE `sha256` = ?", m.schemaHash).Scan(&finishedAt))
		assert.NotNil(t, finishedAt)

		queries, err := sqliteDiff(ctx, db, sqliteSchemaV1)
		require.NoError(t, err)
		assert.Empty(t, queries)

		m, err = NewSQLiteMigration(db, sqliteSchemaV1+"\n-- The same schema\n")
		require.NoError(t, err)
		require.NoError(t, m.Execute(ctx, true))
	})

	t.Run("Change", func(t *testing.T) {
		db := openSQLite(t)

		m, err := NewSQLiteMigration(db, sqliteSchemaV1)
		require.NoError(t, err)
		require.NoError(t, m.Execute(ctx, true))
		_, err = db.Exec("INSERT INTO `user` (`name`, `age`, `legacy`) VALUES ('alice', 20, 'x'), ('bob', NULL, 'y')")
		require.NoError(t, err)
		_, err = db.Exec("INSERT INTO `tag` (`name`) VALUES ('go')")
		require.NoError(t, err)

		m, err = NewSQLiteMigration(db, sqliteSchemaV2)
		require.NoError(t, err)
		require.NoError(t, m.Execute(ctx, false))
		assert.ElementsMatch(t, []string{"user", "obsolete", "tag", "schema_version"}, sqliteObjects(t, db, "table"))

		require.NoError(t, m.Execute(ctx, true))
		assert.ElementsMatch(t, []string{"user", "tag", "blog", "schema_version"}, sqliteObjects(t, db, "table"))
		assert.ElementsMatch(t, []string{"idx_name", "idx_age", "idx_tag_name", "idx_title"}, sqliteObjects(t, db, "index"))

		rows, err := db.Query("SELECT `id`, `name`, `age`, `email`, `score`, `created_at` FROM `user` ORDER BY `id`")
		require.NoError(t, err)
		type user struct {
			id        int64
			name      string
			age       *string
			email     string
			score     float64
			createdAt time.Time
		}
		var users []user
		for rows.Next() {
			var u user
			require.NoError(t, rows.Scan(&u.id, &u.name, &u.age, &u.email, &u.score, &u.createdAt))
			users = append(users, u)
		}
		require.NoError(t, rows.Err())
		require.Len(t, users, 2)
		assert.Equal(t, int64(1), users[0].id)
		assert.Equal(t, "alice", users[0].name)
		require.NotNil(t, users[0].age)
		assert.Equal(t, "20", *users[0].age)
		assert.Equal(t, "", users[0].email)
		assert.Equal(t, 1.5, users[0].score)
		assert.True(t, users[0].createdAt.IsZero())
		assert.Nil(t, users[1].age)

		_, err = db.Exec("INSERT INTO `user` (`name`, `email`, `created_at`) VALUES ('carol', 'carol@example.com', ?)", time.Now())
		require.NoError(t, err)
		var id int64
		require.NoError(t, db.QueryRow("SELECT `id` FROM `user` WHERE `name` = 'carol'").Scan(&id))
		assert.Equal(t, int64(3), id)
		_, err = db.Exec("INSERT INTO `tag` (`name`) VALUES ('go')")
		require.Error(t, err)

		queries, err := sqliteDiff(ctx, db, sqliteSchemaV2)
		require.NoError(t, err)
		assert.Empty(t, queries)
	})

	t.Run("Rollback", func(t *testing.T) {
		db := openSQLite(t)

		m, err := NewSQLiteMigration(db, sqliteSchemaV1)
		require.NoError(t, err)
		require.NoError(t, m.Execute(ctx, true))
		_, err = db.Exec("INSERT INTO `user` (`name`) VALUES ('alice'), ('alice')")
		require.NoError(t, err)

		m, err = NewSQLiteMigration(db, sqliteSchemaV2)
		require.NoError(t, err)
		require.Error(t, m.Execute(ctx, true))
		assert.ElementsMatch(t, []string{"user", "obsolete", "tag", "schema_version"}, sqliteObjects(t, db, "table"))
		var n int
		require.NoError(t, db.QueryRow("SELECT COUNT(*) FROM `schema_version` WHERE `sha256` = ?", m.schemaHash).Scan(&n))
		assert.Equal(t, 0, n)
	})
}

func openSQLite(t *testing.T) *sql.DB {
	t.Helper()

	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "test.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })
	return db
}

func sqliteObjects(t *testing.T, db *sql.DB, typ string) []string {
	t.Helper()

	rows, err := db.Query("SELECT `name` FROM `sqlite_master` WHERE `type` = ? AND `name` NOT LIKE 'sqlite_%'", typ)
	require.NoError(t, err)
	defer rows.Close()
	var names []string
	for rows.Next() {
		var name string
		require.NoError(t, rows.Scan(&name))
		names = append(names, name)
	}
	require.NoError(t, rows.Err())
	return names
}
