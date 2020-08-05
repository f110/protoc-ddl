package schema

import (
	"testing"
)

func TestRelations_Replace(t *testing.T) {
	user := &Field{Name: "user", Virtual: true}
	human := &Field{Name: "human", Virtual: true}
	humanId := &Field{Name: "id", Type: "TYPE_INT32"}
	rels := Relations(make(map[*Field][]*Field))
	rels.Replace(user, human)
	t.Log(rels)

	rels.Replace(human, humanId)
	t.Log(rels)
}
