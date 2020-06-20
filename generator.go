package ddl

import "bytes"

type Generator interface {
	Generate(*bytes.Buffer, []*Table)
}
