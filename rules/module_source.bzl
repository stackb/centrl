"provides the module_source rule"

load("@build_stack_rules_proto//rules:starlark_library.bzl", "StarlarkLibraryFileInfo")
load("//rules:providers.bzl", "ModuleSourceInfo")

def _module_source_impl(ctx):
    return [
        ModuleSourceInfo(
            url = ctx.attr.url,
            url_status_code = ctx.attr.url_status_code,
            url_status_message = ctx.attr.url_status_message,
            integrity = ctx.attr.integrity,
            strip_prefix = ctx.attr.strip_prefix,
            patch_strip = ctx.attr.patch_strip,
            patches = ctx.attr.patches,
            source_json = ctx.file.source_json,
            docs = ctx.files.docs,
            docs_url = ctx.attr.docs_url,
            starlarklibrary = ctx.attr.bzl_srcs[StarlarkLibraryFileInfo],
            docs_url_status_code = ctx.attr.docs_url_status_code,
            docs_url_status_message = ctx.attr.docs_url_status_message,
        ),
    ]

module_source = rule(
    implementation = _module_source_impl,
    attrs = {
        "url": attr.string(),
        "url_status_code": attr.int(doc = "HTTP status code of the url, if present"),
        "url_status_message": attr.string(doc = "HTTP status message of the url, if present"),
        "integrity": attr.string(),
        "strip_prefix": attr.string(),
        "patch_strip": attr.int(default = 0),
        "patches": attr.string_dict(),
        "docs": attr.label_list(allow_files = True),
        "bzl_srcs": attr.label(
            providers = [StarlarkLibraryFileInfo],
            default = "//data/starlark:empty",
        ),
        "docs_url": attr.string(),
        "docs_url_status_code": attr.int(doc = "HTTP status code of the docs_url, if present"),
        "docs_url_status_message": attr.string(doc = "HTTP status message of the docs_url, if present"),
        "source_json": attr.label(allow_single_file = [".json"], mandatory = True),
    },
    provides = [ModuleSourceInfo],
)
