load("@rules_proto//proto:defs.bzl", "ProtoInfo")
load("@bazel_skylib//lib:paths.bzl", "paths")
load("@bazel_skylib//lib:shell.bzl", "shell")
load("@rules_go//go:def.bzl", "GoSource", "go_context")

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

    hash_out = None
    extra = []
    extra_out = []
    if ctx.attr.with_hash:
        if ctx.attr.lang == "txt":
            hash_out = ctx.actions.declare_file("%s.hash.txt" % name)

        if ctx.attr.lang == "go":
            go = go_context(ctx)
            hash_out = ctx.actions.declare_file("%s.hash.go" % name)

        args = ctx.actions.args()
        args.add("--package=" + paths.basename(ctx.attr.importpath))
        args.add("--lang=" + ctx.attr.lang)
        args.add("--outfile=" + hash_out.path)
        args.add(out.path)

        ctx.actions.run(
            executable = ctx.executable._schema_hash,
            inputs = depset(direct = [out]),
            outputs = [hash_out],
            arguments = [args],
        )

        extra_out.append(hash_out)
        if ctx.attr.lang == "go":
            library = go.new_library(go, srcs = [hash_out])
            source = go.library_to_source(go, {}, library, ctx.coverage_instrumented())
            extra += [library, source]

    return [
        DefaultInfo(
            files = depset([out] + extra_out),
        ),
        OutputGroupInfo(
            schema = [out],
            hash = [hash_out],
        ),
    ] + extra

sql_schema = rule(
    implementation = _sql_schema_impl,
    output_to_genfiles = True,
    attrs = {
        "proto": attr.label(providers = [ProtoInfo]),
        "dialect": attr.string(),
        "lang": attr.string(
            doc = "A language name of hash file. Currently go and txt is supported.",
        ),
        "importpath": attr.string(
            doc = "The source import path. This attr will be used only if lang is go.",
        ),
        "with_hash": attr.bool(),
        "protoc": attr.label(
            executable = True,
            cfg = "host",
            default = "@protobuf//:protoc",
        ),
        "compiler": attr.label(
            executable = True,
            cfg = "host",
            default = "//cmd/protoc-gen-ddl",
        ),
        "_well_known_protos": attr.label(
            default = "@protobuf//:well_known_type_protos",
            allow_files = True,
        ),
        "_schema_hash": attr.label(
            executable = True,
            cfg = "host",
            default = "//rules/tools/schema_hash",
        ),
        "_go_context_data": attr.label(
            default = "@rules_go//:go_context_data",
        ),
    },
    toolchains = ["@rules_go//go:toolchain"],
)

def _vendor_ddl_impl(ctx):
    generated = []
    if GoSource in ctx.attr.src:
        generated += [x for x in ctx.attr.src[GoSource].srcs if not x in generated]

    if OutputGroupInfo in ctx.attr.src:
        if "schema" in ctx.attr.src[OutputGroupInfo]:
            generated += [x for x in ctx.attr.src[OutputGroupInfo].schema.to_list() if not x in generated]
        if "hash" in ctx.attr.src[OutputGroupInfo]:
            generated += [x for x in ctx.attr.src[OutputGroupInfo].hash.to_list() if not x in generated]

    substitutions = {
        "@@FROM@@": shell.array_literal([x.path for x in generated]),
        "@@TO@@": shell.quote(ctx.attr.dir),
    }
    out = ctx.actions.declare_file(ctx.label.name + ".sh")
    ctx.actions.expand_template(
        template = ctx.file._template,
        output = out,
        substitutions = substitutions,
        is_executable = True,
    )
    runfiles = ctx.runfiles(files = generated)
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

    extra = []
    if ctx.attr.lang == "go":
        go = go_context(ctx)
        library = go.new_library(go, srcs = [out])
        source = go.library_to_source(go, {}, library, ctx.coverage_instrumented())
        extra += [library, source]

    return [
        DefaultInfo(
            files = depset([out]),
        ),
        OutputGroupInfo(
            schema = [out],
        ),
    ] + extra

schema_entity = rule(
    implementation = _schema_entity_impl,
    output_to_genfiles = True,
    attrs = {
        "proto": attr.label(providers = [ProtoInfo]),
        "lang": attr.string(mandatory = True),
        "protoc": attr.label(
            executable = True,
            cfg = "host",
            default = "@protobuf//:protoc",
        ),
        "compiler": attr.label(
            executable = True,
            cfg = "host",
            default = "//cmd/protoc-gen-entity",
        ),
        "_well_known_protos": attr.label(
            default = "@protobuf//:well_known_type_protos",
            allow_files = True,
        ),
        "_go_context_data": attr.label(
            default = "@rules_go//:go_context_data",
        ),
    },
    toolchains = ["@rules_go//go:toolchain"],
)

def _schema_dao_impl(ctx):
    name, ext = paths.split_extension(ctx.attr.proto[ProtoInfo].direct_sources[0].basename)
    out = ctx.actions.declare_file("%s.dao.go" % name)

    args = ctx.actions.args()
    args.add("--dao_opt", ("lang=%s" % ctx.attr.lang))
    args.add("--dao_out", ("%s:." % out.path))

    _execute_protoc(
        ctx,
        ctx.executable.protoc,
        "dao",
        ctx.executable.compiler,
        ctx.attr.proto,
        args,
        out,
        ctx.files._well_known_protos,
    )

    extra = []
    if ctx.attr.lang == "go":
        go = go_context(ctx)
        library = go.new_library(go, srcs = [out])
        source = go.library_to_source(go, {}, library, ctx.coverage_instrumented())
        extra += [library, source]

    return [
        DefaultInfo(
            files = depset([out]),
        ),
        OutputGroupInfo(
            schema = [out],
        ),
    ] + extra

schema_dao = rule(
    implementation = _schema_dao_impl,
    output_to_genfiles = True,
    attrs = {
        "proto": attr.label(providers = [ProtoInfo]),
        "lang": attr.string(mandatory = True),
        "protoc": attr.label(
            executable = True,
            cfg = "host",
            default = "@protobuf//:protoc",
        ),
        "compiler": attr.label(
            executable = True,
            cfg = "host",
            default = "//cmd/protoc-gen-dao",
        ),
        "_well_known_protos": attr.label(
            default = "@protobuf//:well_known_type_protos",
            allow_files = True,
        ),
        "_go_context_data": attr.label(
            default = "@rules_go//:go_context_data",
        ),
    },
    toolchains = ["@rules_go//go:toolchain"],
)

def _schema_dao_mock_impl(ctx):
    name, ext = paths.split_extension(ctx.attr.proto[ProtoInfo].direct_sources[0].basename)
    out = ctx.actions.declare_file("%s.mock.go" % name)

    args = ctx.actions.args()
    args.add("--dao-mock_opt", ("lang=%s" % ctx.attr.lang))
    args.add("--dao-mock_opt", ("daopath=%s" % ctx.attr.daopath))
    args.add("--dao-mock_out", ("%s:." % out.path))

    _execute_protoc(
        ctx,
        ctx.executable.protoc,
        "dao-mock",
        ctx.executable.compiler,
        ctx.attr.proto,
        args,
        out,
        ctx.files._well_known_protos,
    )

    extra = []
    if ctx.attr.lang == "go":
        go = go_context(ctx)
        library = go.new_library(go, srcs = [out])
        source = go.library_to_source(go, {}, library, ctx.coverage_instrumented())
        extra += [library, source]

    return [
        DefaultInfo(
            files = depset([out]),
        ),
        OutputGroupInfo(
            schema = [out],
        ),
    ] + extra

schema_dao_mock = rule(
    implementation = _schema_dao_mock_impl,
    output_to_genfiles = True,
    attrs = {
        "proto": attr.label(providers = [ProtoInfo]),
        "lang": attr.string(mandatory = True),
        "daopath": attr.string(),
        "protoc": attr.label(
            executable = True,
            cfg = "host",
            default = "@protobuf//:protoc",
        ),
        "compiler": attr.label(
            executable = True,
            cfg = "host",
            default = "//cmd/protoc-gen-dao-mock",
        ),
        "_well_known_protos": attr.label(
            default = "@protobuf//:well_known_type_protos",
            allow_files = True,
        ),
        "_go_context_data": attr.label(
            default = "@rules_go//:go_context_data",
        ),
    },
    toolchains = ["@rules_go//go:toolchain"],
)
