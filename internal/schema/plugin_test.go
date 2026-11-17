package schema

import (
	"slices"
	"strings"
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

func TestMessage_AddQuery(t *testing.T) {
	m := &Message{}
	m.AddQuery("All", "SELECT * FROM `:table_name:`")
	m.AddQuery("ByStatus", "SELECT `status`, COUNT(*) FROM `:table_name:` GROUP BY `status`")
	m.AddQuery("Total", "SELECT COUNT(*) FROM `:table_name:`")
	m.AddQuery("Invalid", "SELECT FROM")

	names := func(queries []*Query) []string {
		res := make([]string, 0, len(queries))
		for _, v := range queries {
			res = append(res, v.Name)
		}
		return res
	}
	if got := names(m.SelectQueries); !slices.Equal(got, []string{"All", "Invalid"}) {
		t.Errorf("unexpected SelectQueries: %v", got)
	}
	if got := names(m.CountQueries); !slices.Equal(got, []string{"ByStatus", "Total"}) {
		t.Errorf("unexpected CountQueries: %v", got)
	}
}

func TestMessages_EachEnum(t *testing.T) {
	enums := make([]*Enum, 0)
	for _, name := range []string{"Zeta", "Alpha", "Mu", "Beta", "Omega", "Gamma", "Delta", "Epsilon", "Eta", "Theta"} {
		enums = append(enums, &Enum{FullName: ".sample." + name})
	}
	msgs := NewMessages(nil, enums)

	expected := []string{"Alpha", "Beta", "Delta", "Epsilon", "Eta", "Gamma", "Mu", "Omega", "Theta", "Zeta"}
	for i := 0; i < 10; i++ {
		got := make([]string, 0)
		msgs.EachEnum(func(e *Enum) {
			got = append(got, strings.TrimPrefix(e.FullName, ".sample."))
		})
		if !slices.Equal(got, expected) {
			t.Fatalf("expected %v: got %v", expected, got)
		}
	}
}
