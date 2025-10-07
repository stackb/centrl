"provides the module_presubmit rule"

load("//rules:providers.bzl", "ModulePresubmitInfo")

def _module_presubmit_impl(ctx):
    return [
        ModulePresubmitInfo(
            presubmit_yml = ctx.file.presubmit_yml,
        ),
    ]

module_presubmit = rule(
    implementation = _module_presubmit_impl,
    attrs = {
        "presubmit_yml": attr.label(allow_single_file = [".yml", ".yaml"]),
    },
    provides = [ModulePresubmitInfo],
)
