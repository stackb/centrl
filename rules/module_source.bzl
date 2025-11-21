"provides the module_source rule"

load("//rules:providers.bzl", "ModuleSourceInfo")

def _compile_stardoc_action(ctx):
    if not ctx.files.docs:
        return None

    output = ctx.actions.declare_file(ctx.label.name + ".docs.pb")

    args = ctx.actions.args()
    args.add("--output_file")
    args.add(output)
    args.add_all(ctx.files.docs)

    ctx.actions.run(
        executable = ctx.executable._documentationcompiler,
        arguments = [args],
        inputs = ctx.files.docs,
        outputs = [output],
        mnemonic = "CompileDocumenatationInfo",
    )

    return output

def _module_source_impl(ctx):
    documentation_info = _compile_stardoc_action(ctx)

    return [
        OutputGroupInfo(
            documentation_info = depset([documentation_info] if documentation_info else []),
        ),
        ModuleSourceInfo(
            url = ctx.attr.url,
            integrity = ctx.attr.integrity,
            strip_prefix = ctx.attr.strip_prefix,
            patch_strip = ctx.attr.patch_strip,
            patches = ctx.attr.patches,
            docs = ctx.files.docs,
            docs_url = ctx.attr.docs_url,
            documentation_info = documentation_info,
            source_json = ctx.file.source_json,
        ),
    ]

module_source = rule(
    implementation = _module_source_impl,
    attrs = {
        "url": attr.string(),
        "integrity": attr.string(),
        "strip_prefix": attr.string(),
        "patch_strip": attr.int(default = 0),
        "patches": attr.string_dict(),
        "docs": attr.label_list(allow_files = True),
        "docs_url": attr.string(),
        "source_json": attr.label(allow_single_file = [".json"], mandatory = True),
        "_documentationcompiler": attr.label(
            default = "//cmd/documentationcompiler",
            executable = True,
            cfg = "exec",
        ),
    },
    provides = [ModuleSourceInfo],
)
