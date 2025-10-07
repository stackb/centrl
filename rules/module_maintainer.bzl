"provides the module_maintainer rule"

load("//rules:providers.bzl", "ModuleMaintainerInfo")

def _module_maintainer_impl(ctx):
    return [
        ModuleMaintainerInfo(
            email = ctx.attr.email,
            username = ctx.attr.name,
            github = ctx.attr.github,
            do_not_notify = ctx.attr.do_not_notify,
            github_user_id = ctx.attr.github_user_id,
        ),
    ]

module_maintainer = rule(
    implementation = _module_maintainer_impl,
    attrs = {
        "email": attr.string(),
        "username": attr.string(),
        "github": attr.string(),
        "do_not_notify": attr.bool(),
        "github_user_id": attr.int(),
    },
    provides = [ModuleMaintainerInfo],
)
