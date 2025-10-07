"provides the module_attestations rule"

load("//rules:providers.bzl", "ModuleAttestationsInfo")

def _module_attestations_impl(ctx):
    return [
        ModuleAttestationsInfo(
            media_type = ctx.attr.media_type,
            urls = ctx.attr.urls,
            integrities = ctx.attr.integrities,
            attestations_json = ctx.file.attestations_json,
        ),
    ]

module_attestations = rule(
    implementation = _module_attestations_impl,
    attrs = {
        "media_type": attr.string(),
        "urls": attr.string_dict(),
        "integrities": attr.string_dict(),
        "attestations_json": attr.label(allow_single_file = [".json"]),
    },
    provides = [ModuleAttestationsInfo],
)
