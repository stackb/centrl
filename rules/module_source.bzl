"provides the module_source rule"

load("//rules:providers.bzl", "ModuleSourceInfo")

def _module_source_impl(ctx):
    return [
        ModuleSourceInfo(
            url = ctx.attr.url,
            integrity = ctx.attr.integrity,
            strip_prefix = ctx.attr.strip_prefix,
            patch_strip = ctx.attr.patch_strip,
            patches = ctx.attr.patches,
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
        "source_json": attr.label(allow_single_file = [".json"], mandatory = True),
    },
    provides = [ModuleSourceInfo],
)
