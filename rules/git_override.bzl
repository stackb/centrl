"provides the git_override rule"

load("//rules:providers.bzl", "GitOverrideInfo", "ModuleOverrideInfo")

def _git_override_impl(ctx):
    return [
        ModuleOverrideInfo(
            module_name = ctx.attr.module_name,
        ),
        GitOverrideInfo(
            module_name = ctx.attr.module_name,
            commit = ctx.attr.commit,
            remote = ctx.attr.remote,
            branch = ctx.attr.branch,
            patch_strip = ctx.attr.patch_strip,
            patches = ctx.attr.patches,
        ),
    ]

git_override = rule(
    implementation = _git_override_impl,
    attrs = {
        "module_name": attr.string(mandatory = True),
        "commit": attr.string(),
        "remote": attr.string(),
        "branch": attr.string(),
        "patch_strip": attr.int(default = 0),
        "patches": attr.string_list(),
    },
    provides = [ModuleOverrideInfo, GitOverrideInfo],
)
