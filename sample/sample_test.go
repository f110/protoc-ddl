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
		changedColumns := blog.ChangedColumn()
		if len(changedColumns) != 1 {
			t.Errorf("Expect return 1 column: %d", len(changedColumns))
		}
	})

	t.Run("Change bytes", func(t *testing.T) {
		blog := &Blog{Id: 1}
		blog.ResetMark()

		blog.Attach = []byte("test")
		changedColumns := blog.ChangedColumn()
		if len(changedColumns) != 1 {
			t.Errorf("Expect return 1 column: %d", len(changedColumns))
		}
	})
}
