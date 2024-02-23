package main

import (
	"bytes"
	"os"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/pluginpb"

	"go.f110.dev/protoc-ddl/internal/generator"
	"go.f110.dev/protoc-ddl/internal/schema"
)

func main() {
	req, err := schema.ParseInput(os.Stdin)
	if err != nil {
		panic(err)
	}
	opt, fileOpt, messages := schema.ProcessEntity(req)

	var res pluginpb.CodeGeneratorResponse
	buf := new(bytes.Buffer)

	switch opt.Lang {
	case "go":
		generator.GoDAOGenerator{}.Generate(buf, fileOpt, messages)
	}

	res.File = append(res.File, &pluginpb.CodeGeneratorResponse_File{
		Name:    proto.String(opt.OutputFile),
		Content: proto.String(buf.String()),
	})

	output, err := proto.Marshal(&res)
	if err != nil {
		panic(err)
	}
	os.Stdout.Write(output)
}
