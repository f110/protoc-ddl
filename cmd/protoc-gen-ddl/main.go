package main

import (
	"os"

	"github.com/f110/protoc-gen-ddl"
	"github.com/golang/protobuf/proto"
)

func main() {
	req, err := genddl.ParseInput()
	if err != nil {
		panic(err)
	}
	res := genddl.Process(req)

	buf, err := proto.Marshal(res)
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(buf)
}
