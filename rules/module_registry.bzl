"provides the module_registry rule"

load("//rules:providers.bzl", "ModuleDependencyCycleInfo", "ModuleMetadataInfo", "ModuleRegistryInfo")

def _compile_action(ctx, modules):
    # Declare output file for compiled proto
    proto_out = ctx.actions.declare_file(ctx.label.name + ".registry.pb")

    # Build arguments for the compiler
    args = ctx.actions.args()
    args.add("--output_file")
    args.add(proto_out)
    args.add_all(modules)

    # Collect all input files
    inputs = modules

    # Run the compiler action
    ctx.actions.run(
        executable = ctx.executable._compiler,
        arguments = [args],
        inputs = inputs,
        outputs = [proto_out],
        mnemonic = "CompileRegistry",
        progress_message = "Compiling registry for %{label}",
    )

    return proto_out

def _module_registry_impl(ctx):
    deps = [d[ModuleMetadataInfo] for d in ctx.attr.deps]
    cycles = [d[ModuleDependencyCycleInfo] for d in ctx.attr.cycles]
    modules = [d.proto for d in deps]

    proto_out = _compile_action(ctx, modules)

    return [
        DefaultInfo(files = depset([proto_out])),
        ModuleRegistryInfo(
            deps = depset(deps),
            cycles = depset(cycles),
            proto = proto_out,
        ),
    ]

module_registry = rule(
    implementation = _module_registry_impl,
    attrs = {
        "deps": attr.label_list(providers = [ModuleMetadataInfo]),
        "cycles": attr.label_list(providers = [ModuleDependencyCycleInfo]),
        "_compiler": attr.label(
            default = "//cmd/registrycompiler",
            executable = True,
            cfg = "exec",
        ),
    },
    provides = [ModuleRegistryInfo],
)
