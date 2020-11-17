package main

import (
	"bytes"
	"os"

	"github.com/golang/protobuf/proto"
	plugin_go "github.com/golang/protobuf/protoc-gen-go/plugin"

	"go.f110.dev/protoc-ddl/internal/generator"
	"go.f110.dev/protoc-ddl/internal/schema"
)

func main() {
	req, err := schema.ParseInput(os.Stdin)
	if err != nil {
		panic(err)
	}
	opt, fileOpt, messages := schema.ProcessEntity(req)

	var res plugin_go.CodeGeneratorResponse
	buf := new(bytes.Buffer)

	switch opt.Lang {
	case "go":
		generator.GoDAOMockGenerator{}.Generate(buf, fileOpt, messages)
	}

	res.File = append(res.File, &plugin_go.CodeGeneratorResponse_File{
		Name:    proto.String(opt.OutputFile),
		Content: proto.String(buf.String()),
	})

	output, err := proto.Marshal(&res)
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(output)
}
