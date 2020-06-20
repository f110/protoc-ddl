load("@bazel_skylib//lib:shell.bzl", "shell")

def _vendor_proto_source_impl(ctx):
    generated = ctx.attr.src[OutputGroupInfo].go_generated_srcs.to_list()
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

_vendor_proto_source = rule(
    implementation = _vendor_proto_source_impl,
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

def vendor_proto_source(name, **kwargs):
    if not "dir" in kwargs:
        dir = native.package_name()
        kwargs["dir"] = dir

    _vendor_proto_source(name = name, **kwargs)
