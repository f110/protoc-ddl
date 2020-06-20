package main

import (
	"os"

	"github.com/golang/protobuf/proto"

	"go.f110.dev/protoc-ddl/internal/generator"
)

func main() {
	req, err := generator.ParseInput(os.Stdin)
	if err != nil {
		panic(err)
	}
	res := generator.Process(req)

	buf, err := proto.Marshal(res)
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(buf)
}
