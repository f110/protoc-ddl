load("@rules_proto//proto:defs.bzl", "ProtoInfo")
load("@bazel_skylib//lib:paths.bzl", "paths")
load("@bazel_skylib//lib:shell.bzl", "shell")

def _execute_protoc(ctx, protoc, lang_name, plugin, proto, args, out, well_known_protos):
    proto = proto[ProtoInfo]

    args.add("--plugin", ("protoc-gen-%s=%s" % (lang_name, plugin.path)))
    args.add_all(proto.transitive_proto_path, format_each = "--proto_path=%s")

    proto_files = []
    for i in proto.direct_sources:
        args.add(i.path)
        proto_files.append(i)

    ctx.actions.run(
        executable = protoc,
        tools = [plugin],
        inputs = depset(
            direct = proto_files,
            transitive = [depset(
                direct = well_known_protos,
                transitive = [proto.transitive_sources],
            )],
        ),
        outputs = [out],
        arguments = [args],
    )

def _sql_schema_impl(ctx):
    dialect = ctx.attr.dialect
    if not dialect:
        dialect = "mysql"
    name, ext = paths.split_extension(ctx.attr.proto[ProtoInfo].direct_sources[0].basename)
    out = ctx.actions.declare_file("%s.sql" % name)

    args = ctx.actions.args()
    args.add("--ddl_opt", ("dialect=%s" % dialect))
    args.add("--ddl_out", ("%s:." % out.path))

    _execute_protoc(
        ctx,
        ctx.executable.protoc,
        "ddl",
        ctx.executable.compiler,
        ctx.attr.proto,
        args,
        out,
        ctx.files._well_known_protos,
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

def _vendor_ddl_impl(ctx):
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

_vendor_ddl = rule(
    implementation = _vendor_ddl_impl,
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

def vendor_ddl(name, **kwargs):
    if not "dir" in kwargs:
        dir = native.package_name()
        kwargs["dir"] = dir

    _vendor_ddl(name = name, **kwargs)

def _schema_entity_impl(ctx):
    name, ext = paths.split_extension(ctx.attr.proto[ProtoInfo].direct_sources[0].basename)
    out = ctx.actions.declare_file("%s.entity.go" % name)

    args = ctx.actions.args()
    args.add("--entity_opt", ("lang=%s" % ctx.attr.lang))
    args.add("--entity_out", ("%s:." % out.path))

    _execute_protoc(
        ctx,
        ctx.executable.protoc,
        "entity",
        ctx.executable.compiler,
        ctx.attr.proto,
        args,
        out,
        ctx.files._well_known_protos,
    )

    return [
        DefaultInfo(
            files = depset([out]),
        ),
        OutputGroupInfo(
            schema = [out],
        ),
    ]

schema_entity = rule(
    implementation = _schema_entity_impl,
    output_to_genfiles = True,
    attrs = {
        "proto": attr.label(providers = [ProtoInfo]),
        "lang": attr.string(mandatory = True),
        "protoc": attr.label(
            executable = True,
            cfg = "host",
            default = "@com_google_protobuf//:protoc",
        ),
        "compiler": attr.label(
            executable = True,
            cfg = "host",
            default = "//cmd/protoc-gen-entity",
        ),
        "_well_known_protos": attr.label(
            default = "@com_google_protobuf//:well_known_protos",
            allow_files = True,
        ),
    },
)