package schema

import (
	"testing"
)

func TestMessages_Denormalize(t *testing.T) {
	msgs := []*Message{
		{
			Package: ".test",
			Fields: []*Field{
				{Name: "id", Type: ""},
			},
		},
	}
	t.Log(msgs)
}
