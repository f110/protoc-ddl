protoc-ddl
---

**THIS PROJECT IS WORKING IN PROGRESS**

protoc-ddl is a tool of define and generate the schema for RDB.

# How to use

## Command Line

```
$ go get go.f110.dev/protoc-ddl/cmd/protoc-gen-ddl
```

## With Bazel (Bzlmod)

`MODULE.bazel`

```starlark
bazel_dep(name = "protoc_ddl", version = "1.0")
bazel_dep(name = "rules_go", version = "0.57.0")
bazel_dep(name = "rules_proto", version = "7.1.0")
bazel_dep(name = "protobuf", version = "32.1")

# protoc_ddl is not yet published to Bazel Central Registry.
# Pin it to a specific revision via git_override.
git_override(
    module_name = "protoc_ddl",
    remote = "https://github.com/f110/protoc-ddl.git",
    commit = "<commit hash>",
)
```

`BUILD.bazel`

```starlark
load("@rules_proto//proto:defs.bzl", "proto_library")
load("@rules_go//go:def.bzl", "go_library")
load("@rules_go//proto:def.bzl", "go_proto_library")
load("@protoc_ddl//rules:def.bzl", "sql_schema", "vendor_ddl")

proto_library(
    name = "database_proto",
    srcs = ["schema.proto"],
    visibility = ["//visibility:public"],
    deps = [
        "@protoc_ddl//:ddl_proto",
        "@protobuf//:timestamp_proto",
    ],
)

go_proto_library(
    name = "database_go_proto",
    importpath = "example.com/database",
    proto = ":database_proto",
    visibility = ["//visibility:public"],
    deps = ["@protoc_ddl//:protoc-ddl"],
)

go_library(
    name = "database",
    embed = [":database_go_proto"],
    importpath = "example.com/database",
    visibility = ["//visibility:public"],
)

sql_schema(
    name = "schema",
    proto = ":database_proto",
)

vendor_ddl(
    name = "vendor_schema",
    src = ":schema",
)
```

You can see the generated schema file by the following command.

```
$ bazel run //:vendor_schema
```

You will see the generated schema file in the same directory at `*.sql`.

# Migration

This tool also supports the migration.

```
$ migrate --schema ./schema.sql --driver mysql --dsn "root@tcp(localhost)/protoc_ddl" --execute
```

Currently, Only mysql is supported.