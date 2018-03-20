package genddl

import "bytes"

type Dialect interface {
	Generate(*bytes.Buffer, []Table)
}
