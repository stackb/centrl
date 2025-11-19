"provides the octicons_closure_template_library rule"

def _compile(ctx, octicons_json):
    output = ctx.actions.declare_file(ctx.label.name + ".soy")

    args = ctx.actions.args()
    args.add("--output_file")
    args.add(output)
    args.add("--octicons_json_file")
    args.add(octicons_json)

    ctx.actions.run(
        executable = ctx.executable._octiconcompiler,
        arguments = [args],
        inputs = [octicons_json],
        outputs = [output],
        mnemonic = "OcticonsClosureTemplateCompile",
        progress_message = "Compiling " + output.basename,
    )

    return output

def _executable(ctx, output):
    ctx.actions.write(
        ctx.outputs.executable,
        "cp -f {output} $BUILD_WORKSPACE_DIRECTORY/{package}".format(
            output = output.short_path,
            package = ctx.label.package,
        ),
        is_executable = True,
    )

def _octicons_closure_template_compile_impl(ctx):
    octicons_soy = _compile(ctx, ctx.file._octicons_json)
    _executable(ctx, octicons_soy)

    return [
        DefaultInfo(
            executable = ctx.outputs.executable,
            files = depset([octicons_soy]),
            runfiles = ctx.runfiles(files = [octicons_soy]),
        ),
    ]

octicons_closure_template_compile = rule(
    implementation = _octicons_closure_template_compile_impl,
    attrs = {
        "_octicons_json": attr.label(
            default = "//data:octicons.json",
            allow_single_file = True,
        ),
        "_octiconcompiler": attr.label(
            default = "//cmd/octiconcompiler",
            executable = True,
            cfg = "exec",
        ),
    },
    executable = True,
)
