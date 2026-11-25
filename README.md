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

## Dialect

`sql_schema` generates the schema for MySQL by default. Set `dialect = "sqlite"` to generate the schema for SQLite.
The command line plugin takes the dialect by `--ddl_opt dialect=sqlite`.

SQLite has some limitations.

- `sequence` is allowed only for the column which is the only primary key. The column becomes `INTEGER PRIMARY KEY AUTOINCREMENT`.
- The name of the index must be unique in the database, not in the table.

# Query

The queries of DAO are defined by `dev.f110.ddl.dao`. The conditions of WHERE clause can use `LIKE ?`, `IN (?)` and
parentheses in addition to the comparison operators.

`IN (?)` takes a slice. If the slice is empty, the condition is always false.

```
queries: {
  name: "Search"
  query: "SELECT * FROM `:table_name:` WHERE `status` IN (?) AND (`title` LIKE ? OR `body` LIKE ?)"
}
```

The above query generates `ListSearch(ctx context.Context, status []uint32, title string, body string, opt ...ListOption)`.

## Count

The query which selects `COUNT(*)` generates `Count<Name>`. It returns `int64` without GROUP BY.

With GROUP BY, the columns must be the same as GROUP BY, and the struct `<Message><Name>Count` which has the columns and
`Count` is generated in the entity package.

```
queries: {
  name: "ByStatus"
  query: "SELECT `status`, COUNT(*) FROM `:table_name:` WHERE `user_id` IN (?) GROUP BY `status`"
}
```

The above query generates `CountByStatus(ctx context.Context, userId []int32) ([]*BlogByStatusCount, error)`.

# Migration

This tool also supports the migration.

```
$ migrate --schema ./schema.sql --driver mysql --dsn "root@tcp(localhost)/protoc_ddl" --execute
$ migrate --schema ./schema.sql --driver sqlite --dsn ./local.db --execute
```

The driver is `mysql` or `sqlite`. The migration of SQLite is executed in a single transaction started by `BEGIN IMMEDIATE`. The table which has the changed columns is recreated and the data is copied
from the old table. The new `NOT NULL` column without the default value is filled with the zero value.