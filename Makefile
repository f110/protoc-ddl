%.pb.go: %.proto
	protoc --go_out=$(GOPATH)/src $^

sample/schema.sql: sample/schema.proto
	protoc --plugin=bin/protoc-gen-ddl --ddl_out=dialect=mysql,$@:. -I=. $^

bin/%: cmd/%/main.go
	go build -o $@ $^

update-deps:
	bazel run //:gazelle -- update