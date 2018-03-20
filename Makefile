%.pb.go: %.proto
	protoc --go_out=$(GOPATH)/src $^

sample/schema.sql: sample/schema.proto
	protoc --plugin=build/protoc-gen-ddl --ddl_out=dialect=mysql:./ -I=. $^

build/%: cmd/%/main.go
	go build -o $@ $^