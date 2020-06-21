load("@rules_proto//proto:defs.bzl", "ProtoInfo")
load("@bazel_skylib//lib:shell.bzl", "shell")

def _sql_schema_impl(ctx):
    dialect = ctx.attr.dialect
    if not dialect:
        dialect = "mysql"
    out = ctx.actions.declare_file("schema.sql")

    proto = ctx.attr.proto[ProtoInfo]

    args = ctx.actions.args()
    args.add(("--plugin=protoc-gen-ddl=%s" % ctx.executable.compiler.path))
    args.add_all(proto.transitive_proto_path, format_each = "--proto_path=%s")
    args.add(("--ddl_out=dialect=%s,%s:.") % (dialect, out.path))

    proto_files = []
    for i in proto.direct_sources:
        args.add(i.path)
        proto_files.append(i)

    ctx.actions.run(
        executable = ctx.executable.protoc,
        tools = [ctx.executable.compiler],
        inputs = depset(
            direct = proto_files,
            transitive = [depset(
                direct = ctx.files._well_known_protos,
                transitive = [proto.transitive_sources],
            )],
        ),
        outputs = [out],
        arguments = [args],
    )
    return [
        DefaultInfo(
            files = depset([out]),
        ),
        OutputGroupInfo(
            schema = [out],
        ),
    ]

sql_schema = rule(
    implementation = _sql_schema_impl,
    output_to_genfiles = True,
    attrs = {
        "proto": attr.label(providers = [ProtoInfo]),
        "dialect": attr.string(),
        "protoc": attr.label(
            executable = True,
            cfg = "host",
            default = "@com_google_protobuf//:protoc",
        ),
        "compiler": attr.label(
            executable = True,
            cfg = "host",
            default = "//cmd/protoc-gen-ddl",
        ),
        "_well_known_protos": attr.label(
            default = "@com_google_protobuf//:well_known_protos",
            allow_files = True,
        ),
    },
)

def _vendor_sql_schema_impl(ctx):
    generated = ctx.attr.src[OutputGroupInfo].schema.to_list()
    substitutions = {
        "@@FROM@@": shell.quote(generated[0].path),
        "@@TO@@": shell.quote(ctx.attr.dir),
    }
    out = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.expand_template(
        template = ctx.file._template,
        output = out,
        substitutions = substitutions,
        is_executable = True,
    )
    runfiles = ctx.runfiles(files = [generated[0]])
    return [
        DefaultInfo(
            runfiles = runfiles,
            executable = out,
        ),
    ]

_vendor_sql_schema = rule(
    implementation = _vendor_sql_schema_impl,
    executable = True,
    attrs = {
        "dir": attr.string(),
        "src": attr.label(),
        "_template": attr.label(
            default = "//build:move-into-workspace.bash",
            allow_single_file = True,
        ),
    },
)

def vendor_sql_schema(name, **kwargs):
    if not "dir" in kwargs:
        dir = native.package_name()
        kwargs["dir"] = dir

    _vendor_sql_schema(name = name, **kwargs)
