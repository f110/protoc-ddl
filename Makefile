.PHONY: sample/schema.sql
sample/schema.sql: sample/schema.proto
	protoc --plugin=bazel-bin/cmd/protoc-gen-ddl/protoc-gen-ddl_/protoc-gen-ddl --ddl_out=dialect=mysql,$@:. -I=. $^

update-deps:
	bazel run //:vendor_proto_source
	bazel run //:gazelle -- update

.PHONY: gen