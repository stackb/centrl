"""starlark plugin definitions"""

def _configure(ctx):
    """_configure prepares the PluginConfiguration for the builtin prost plugin.

    Args:
        ctx (protoc.PluginContext): The context object.
    Returns:
        config (PluginConfiguration): The configured PluginConfiguration object.
    """
    outputs = dict()
    for file in ctx.proto_library.files:
        if len(file.services) == 0:
            continue
        outputs[file.pkg.name + ".tonic.rs"] = None

    if len(outputs) == 0:
        return None

    config = protoc.PluginConfiguration(
        label = "@monosol//bazel_tools/rust:protoc-gen-tonic",
        outputs = outputs.keys(),
        options = ctx.plugin_config.options,
    )

    return config

protoc.Plugin(
    name = "protoc-gen-tonic",
    configure = _configure,
)
