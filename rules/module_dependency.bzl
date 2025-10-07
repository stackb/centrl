"provides the module_dependency rule"

load("//rules:providers.bzl", "ModuleDependencyInfo", "ModuleVersionInfo")

def _module_dependency_impl(ctx):
    return [
        ModuleDependencyInfo(
            name = ctx.attr.dep_name,
            version = ctx.attr.version,
            dev = ctx.attr.dev,
            module = ctx.attr.module,
        ),
    ]

module_dependency = rule(
    implementation = _module_dependency_impl,
    attrs = {
        "dep_name": attr.string(mandatory = True),
        "version": attr.string(),
        "dev": attr.bool(default = False),
        "module": attr.label(providers = [ModuleVersionInfo]),
    },
    provides = [ModuleDependencyInfo],
)
