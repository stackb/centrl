"provides the module_dependency_cycle rule"

load("//rules:providers.bzl", "ModuleDependencyCycleInfo")

def _module_dependency_cycle_impl(ctx):
    return [
        ModuleDependencyCycleInfo(
            modules = ctx.attr.deps,
            cycle_name = ctx.attr.name,
        ),
    ]

module_dependency_cycle = rule(
    implementation = _module_dependency_cycle_impl,
    attrs = {
        "deps": attr.string_list(),
        "cycle_modules": attr.string_list(),
    },
    provides = [ModuleDependencyCycleInfo],
)
