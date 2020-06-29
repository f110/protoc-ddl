package sample

import (
	"testing"
	"time"
)

func TestBlog_ChangedColumn(t *testing.T) {
	t.Run("Add nullable Timestamp", func(t *testing.T) {
		blog := &Blog{Id: 1}
		blog.ResetMark()

		now := time.Now()
		blog.UpdatedAt = &now
		if !blog.IsChanged() {
			t.Fatal("Expect IsChanged is true")
		}
		changedColumns := blog.ChangedColumn()
		if len(changedColumns) != 1 {
			t.Errorf("Expect return 1 column: %d", len(changedColumns))
		}
	})

	t.Run("Add nullable int", func(t *testing.T) {
		blog := &Blog{Id: 1}
		blog.ResetMark()

		var categoryId int32 = 1
		blog.CategoryId = &categoryId
		if !blog.IsChanged() {
			t.Fatal("Expect IsChanged is true")
		}
		changedColumn := blog.ChangedColumn()
		if len(changedColumn) != 1 {
			t.Errorf("Expect return 1 column: %d", len(changedColumn))
		}
	})

	t.Run("Remove nullable int", func(t *testing.T) {
		var categoryId int32 = 1
		blog := &Blog{Id: 1, CategoryId: &categoryId}
		blog.ResetMark()

		blog.CategoryId = nil
		if !blog.IsChanged() {
			t.Fatal("Expect IsChanged is true")
		}
		changedColumn := blog.ChangedColumn()
		if len(changedColumn) != 1 {
			t.Errorf("Expect return 1 column: %d", len(changedColumn))
		}
	})

	t.Run("Change bytes", func(t *testing.T) {
		blog := &Blog{Id: 1}
		blog.ResetMark()

		blog.Attach = []byte("test")
		if !blog.IsChanged() {
			t.Fatal("Expect IsChanged is true")
		}
		changedColumns := blog.ChangedColumn()
		if len(changedColumns) != 1 {
			t.Errorf("Expect return 1 column: %d", len(changedColumns))
		}
	})
}
