package genddl

import "bytes"

type Generator interface {
	Generate(*bytes.Buffer, []Table)
}
