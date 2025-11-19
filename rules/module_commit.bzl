"provides the module_commit rule"

load("//rules:providers.bzl", "ModuleCommitInfo")

def _module_commit_impl(ctx):
    return [
        ModuleCommitInfo(
            sha1 = ctx.attr.sha1,
            date = ctx.attr.date,
            message = ctx.attr.message,
        ),
    ]

module_commit = rule(
    implementation = _module_commit_impl,
    attrs = {
        "sha1": attr.string(mandatory = True, doc = "Git commit SHA-1 hash"),
        "date": attr.string(mandatory = True, doc = "Commit date in ISO 8601 format"),
        "message": attr.string(mandatory = True, doc = "Commit message"),
    },
    provides = [ModuleCommitInfo],
)
