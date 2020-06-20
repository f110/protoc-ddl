package main

import (
	"os"

	"github.com/golang/protobuf/proto"

	"go.f110.dev/protoc-ddl"
)

func main() {
	req, err := ddl.ParseInput()
	if err != nil {
		panic(err)
	}
	res := ddl.Process(req)

	buf, err := proto.Marshal(res)
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(buf)
}
