package dao

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"go.f110.dev/protoc-ddl/sample"
	"go.f110.dev/protoc-ddl/sample/dao/daotest"
)

func TestUser(t *testing.T) {
	u := daotest.NewUser()
	u.RegisterSelect(1, &sample.User{Id: 1, Name: "Foobar"})

	v, err := u.Select(context.TODO(), 1)
	require.NoError(t, err)
	assert.Equal(t, "Foobar", v.Name)

	_, err = u.Create(context.TODO(), &sample.User{Id: 2})
	require.NoError(t, err)
	call := u.Called("Create")
	require.Len(t, call, 1)
	user, ok := call[0].Args["user"].(*sample.User)
	require.True(t, ok)
	assert.Equal(t, int32(2), user.Id)

	err = u.Update(context.TODO(), &sample.User{Id: 2, Name: "Test2"})
	require.NoError(t, err)
	call = u.Called("Update")
	require.Len(t, call, 1)
	user, ok = call[0].Args["user"].(*sample.User)
	require.True(t, ok)
	assert.Equal(t, int32(2), user.Id)
	assert.Equal(t, "Test2", user.Name)

	err = u.Delete(context.TODO(), 1)
	require.NoError(t, err)
	call = u.Called("Delete")
	require.Len(t, call, 1)
	id, ok := call[0].Args["id"].(int32)
	require.True(t, ok)
	assert.Equal(t, int32(1), id)

	b := daotest.NewBlog()
	b.RegisterListByTitle("new", []*sample.Blog{{Id: 1, Title: "new"}, {Id: 2, Title: "new"}})

	l, err := b.ListByTitle(context.TODO(), "new")
	require.NoError(t, err)
	assert.Len(t, l, 2)
	assert.Equal(t, int64(1), l[0].Id)
	assert.Equal(t, int64(2), l[1].Id)
}
