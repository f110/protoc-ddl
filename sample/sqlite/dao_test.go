package sqlite

import (
	"context"
	"database/sql"
	_ "embed"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.f110.dev/protoc-ddl/internal/migrate"
	"go.f110.dev/protoc-ddl/probe"
	"go.f110.dev/protoc-ddl/sample"
	"go.f110.dev/protoc-ddl/sample/dao"
)

//go:embed schema.sql
var schema string

func TestDAO(t *testing.T) {
	ctx := context.Background()
	db, err := sql.Open("sqlite", filepath.Join(t.TempDir(), "sample.db"))
	require.NoError(t, err)
	t.Cleanup(func() { db.Close() })

	m, err := migrate.NewSQLiteMigration(db, schema)
	require.NoError(t, err)
	require.NoError(t, m.Execute(ctx, true))

	users := dao.NewUser(db)
	now := time.Now().Truncate(time.Second)
	user, err := users.Create(ctx, &sample.User{Age: 30, Name: "alice", Title: "writer", Status: sample.UserStatusActive, CreatedAt: now})
	require.NoError(t, err)
	assert.Equal(t, int32(1), user.Id)

	user.Name = "bob"
	require.NoError(t, users.Update(ctx, user))
	got, err := users.Select(ctx, user.Id)
	require.NoError(t, err)
	assert.Equal(t, "bob", got.Name)
	assert.Equal(t, sample.UserStatusActive, got.Status)
	assert.True(t, now.Equal(got.CreatedAt))

	blogs := dao.NewBlog(db)
	var categoryId int32 = 3
	_, err = blogs.Create(ctx, &sample.Blog{UserId: user.Id, Title: "first", CategoryId: &categoryId, Attach: []byte("a"), EditorId: user.Id, Sign: []byte("s"), Status: sample.BlogStatusPublished})
	require.NoError(t, err)
	_, err = blogs.Create(ctx, &sample.Blog{UserId: user.Id, Title: "second", Attach: []byte{}, EditorId: user.Id, Sign: []byte{}, Status: sample.BlogStatusDraft})
	require.NoError(t, err)
	_, err = blogs.Create(ctx, &sample.Blog{UserId: user.Id, Title: "first", Attach: []byte{}, EditorId: user.Id, Sign: []byte{}})
	require.Error(t, err)

	l, err := blogs.ListByTitle(ctx, "first", dao.Limit(1))
	require.NoError(t, err)
	require.Len(t, l, 1)
	assert.Equal(t, []byte("a"), l[0].Attach)
	require.NotNil(t, l[0].CategoryId)
	assert.Equal(t, categoryId, *l[0].CategoryId)
	assert.Nil(t, l[0].UpdatedAt)
	require.NotNil(t, l[0].User)
	assert.Equal(t, "bob", l[0].User.Name)

	n, err := blogs.CountByUser(ctx, user.Id)
	require.NoError(t, err)
	assert.Equal(t, int64(2), n)
	counts, err := blogs.CountByStatus(ctx, []int32{user.Id})
	require.NoError(t, err)
	assert.Len(t, counts, 2)

	require.NoError(t, blogs.Delete(ctx, l[0].Id))
	_, err = blogs.Select(ctx, l[0].Id)
	require.ErrorIs(t, err, sql.ErrNoRows)

	assert.True(t, probe.NewProbe(db).Ready(ctx, schemaHash(t, db)))
}

func schemaHash(t *testing.T, db *sql.DB) string {
	t.Helper()

	var hash string
	require.NoError(t, db.QueryRow("SELECT `sha256` FROM `schema_version`").Scan(&hash))
	return hash
}
