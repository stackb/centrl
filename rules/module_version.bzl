"provides the module_version rule"

load("@build_stack_rules_proto//rules:starlark_library.bzl", "StarlarkLibraryFileInfo")
load("//rules:semver.bzl", "semver_max")
load(
    "//rules:providers.bzl",
    "ModuleAttestationsInfo",
    "ModuleCommitInfo",
    "ModuleDependencyInfo",
    "ModulePresubmitInfo",
    "ModuleSourceInfo",
    "ModuleVersionInfo",
)

def _make_moduleversion_key(name, version):
    if version:
        return "%s:%s" % (name, version)
    return name

def _dep_module_dependency_repo_name(dep):
    return dep.repo_name if dep.repo_name else dep.name

def _make_moduleversion(name, version, deps, starlarklibrary):
    return struct(
        key = _make_moduleversion_key(name, version),
        name = name,
        version = version,
        deps = {_dep_module_dependency_repo_name(dep): dep for dep in deps},
        starlarklibrary = starlarklibrary,
        unknown_loads = [],
    )

def _compile_best_effort_extract_documentation_action(ctx, source, deps, transitive_deps):
    """Prepares the action to extract stardoc for this moduleversion

    Args:
        ctx: the context object
        source: ModuleSourceInfo
        deps: List<ModuleDependencyInfo> - direct deps of this module version
        transitive_deps: DepSet<ModuleDependencyInfo> - direct deps of this module version
    """

    # Uncomment the line below to print the dependency tree for debugging
    # _print_dependency_tree(transitive_deps, ctx.attr.module_name, ctx.attr.version)

    output = ctx.actions.declare_file("%s/modules/%s/%s/documentationinfo.pb" % (ctx.label.name, ctx.attr.module_name, ctx.attr.version))
    # output = ctx.actions.declare_file(ctx.label.name + ".moduleinfo.json")

    # JavaRuntimeInfo - current one
    java_runtime = ctx.attr._java_runtime[java_common.JavaRuntimeInfo]

    # str - to the /bin/java program
    java_executable = java_runtime.java_executable_exec_path

    # List[Struct] - transitive depset of .bzl file Providers
    moduleversions = [
        _make_moduleversion(dep.module.name, dep.module.version, dep.module.deps, dep.module.source.starlarklibrary)
        for dep in transitive_deps.to_list()
        if getattr(dep.module, "source", None)
    ] + [
        _make_moduleversion(ctx.attr.module_name, ctx.attr.version, deps, source.starlarklibrary),
        _make_moduleversion("bazel_tools", "", [], ctx.attr._bazel_tools[StarlarkLibraryFileInfo]),
    ]

    versions_map = {}
    mvs = []
    for mv in moduleversions:
        versions = versions_map.setdefault(mv.name, {})
        versions[mv.version] = mv
    for name in versions_map.keys():
        versions = versions_map[name]
        highest = semver_max(versions.keys())
        mvs.append(versions[highest])

    # DepSet[File] - transitive .bzl source files
    transitive_srcs = [depset(mv.starlarklibrary.srcs) for mv in mvs]

    # DepSet[File] - all file input
    inputs = depset([ctx.file._starlarkserverjar], transitive = transitive_srcs)

    args = ctx.actions.args()
    args.use_param_file("@%s", use_always = True)
    args.set_param_file_format("multiline")

    args.add("--output_file", output)
    args.add("--port", 3524)
    args.add("--java_interpreter_file", java_executable)
    args.add("--server_jar_file", ctx.file._starlarkserverjar)
    args.add("--workspace_cwd", ctx.bin_dir.path)
    args.add("--workspace_output_base", "/private/var/tmp/_bazel_pcj/4d50590a9155e202dda3b0ac2e024c3f")

    for mv in mvs:
        for file in mv.starlarklibrary.srcs:
            args.add("--bzl_file=%s:%s" % (mv.name, file.path))

    for mv in mvs:
        for dep in mv.deps.values():
            args.add("--module_dep=%s:%s=%s" % (mv.name, dep.name, dep.repo_name))

    for file in source.starlarklibrary.srcs:
        args.add(file.path)

    ctx.actions.run(
        mnemonic = "CompileModuleInfo",
        progress_message = "Extracting %s (%d files)" % (str(ctx.label), len(source.starlarklibrary.srcs)),
        execution_requirements = {
            "supports-workers": "1",
            "requires-worker-protocol": "proto",
        },
        executable = ctx.executable._starlarkcompiler,
        arguments = [args],
        inputs = inputs,
        outputs = [output],
        tools = java_runtime.files.to_list(),
    )

    return output

def _status_code_exists(code):
    return code >= 200 and code < 300

def _compile_published_documentation_action(ctx, files):
    if not files:
        return None

    output = ctx.actions.declare_file("%s/modules/%s/%s/documentationinfo.pb" % (ctx.label.name, ctx.attr.module_name, ctx.attr.version))

    args = ctx.actions.args()
    args.add("--output_file")
    args.add(output)
    args.add_all(files)

    ctx.actions.run(
        executable = ctx.executable._documentationcompiler,
        arguments = [args],
        inputs = files,
        outputs = [output],
        mnemonic = "CompileDocumenatationInfo",
    )

    return output

def _compile_docs_action(ctx, source, deps, transitive_deps, unresolved_deps):
    # if the module_source has published / "offical" docs, use those (assuming
    # the doc link isn't broken).
    if len(source.docs) > 0 and _status_code_exists(source.docs_url_status_code):
        return _compile_published_documentation_action(ctx, source.docs)

    # if the module does not publish docs and this is NOT the latest version,
    # skip.
    if not ctx.attr.is_latest_version:
        # print("skipping docgen for %s (not latest version)" % ctx.label)
        return None

    # if we have unresolved dependencies, the best effort method will fail
    if unresolved_deps:
        return None

    # otherwise, attempt to extract docs automatically
    return _compile_best_effort_extract_documentation_action(ctx, source, deps, transitive_deps)

def _compile_action(ctx, source, deps, transitive_deps, attestations, presubmit, commit):
    # Declare output file for compiled proto
    proto_out = ctx.actions.declare_file(ctx.label.name + ".moduleversion.pb")

    # Build arguments for the compiler
    args = ctx.actions.args()
    args.add("--module_bazel_file")
    args.add(ctx.file.module_bazel)
    args.add("--output_file")
    args.add(proto_out)

    # All of the module dependency info is in the MODULE.bazel file, but the
    # 'unresolved' property is discovered during gazelle resolution.  Pass that
    # information into the compiler.
    unresolved_deps = [dep.name for dep in deps if dep.unresolved]
    if unresolved_deps:
        args.add("--unresolved_deps")
        args.add(",".join(unresolved_deps))

    # Collect all input files
    inputs = [ctx.file.module_bazel]

    # Add optional source.json file
    if source and source.source_json:
        args.add("--source_json_file")
        args.add(source.source_json)
        inputs.append(source.source_json)

        args.add("--url_status_code=" + str(source.url_status_code))
        args.add("--url_status_message=" + source.url_status_message)
        args.add("--docs_url_status_code=" + str(source.docs_url_status_code))
        args.add("--docs_url_status_message=" + source.docs_url_status_message)

    # optionally include docs
    docs_out = _compile_docs_action(ctx, source, deps, transitive_deps, unresolved_deps)
    # if docs_out:
    #     args.add("--documentation_info_file")
    #     args.add(docs_out)
    #     inputs.append(docs_out)

    # Add optional presubmit.yml file
    if presubmit and presubmit.presubmit_yml:
        args.add("--presubmit_yml_file")
        args.add(presubmit.presubmit_yml)
        inputs.append(presubmit.presubmit_yml)

    # Add optional attestations.json file
    if attestations and attestations.attestations_json:
        args.add("--attestations_json_file")
        args.add(attestations.attestations_json)
        inputs.append(attestations.attestations_json)

    # Add optional commit metadata
    if commit:
        args.add("--commit_sha1")
        args.add(commit.sha1)
        args.add("--commit_date")
        args.add(commit.date)
        args.add("--commit_message")
        args.add(commit.message)

    # Run the compiler action
    ctx.actions.run(
        executable = ctx.executable._moduleversioncompiler,
        arguments = [args],
        inputs = inputs,
        outputs = [proto_out],
        mnemonic = "CompileModuleVersion",
        progress_message = "Compiling module version for %{label}",
    )

    return struct(
        module = proto_out,
        docs = docs_out,
    )

def _module_version_impl(ctx):
    deps = [dep[ModuleDependencyInfo] for dep in ctx.attr.deps]
    transitive_deps = depset(deps, transitive = [dep.module.transitive_deps for dep in deps if getattr(dep, "module", None)])

    source = ctx.attr.source[ModuleSourceInfo]
    attestations = ctx.attr.attestations[ModuleAttestationsInfo] if ctx.attr.attestations and ModuleAttestationsInfo in ctx.attr.attestations else None
    presubmit = ctx.attr.presubmit[ModulePresubmitInfo] if ctx.attr.presubmit and ModulePresubmitInfo in ctx.attr.presubmit else None
    commit = ctx.attr.commit[ModuleCommitInfo] if ctx.attr.commit and ModuleCommitInfo in ctx.attr.commit else None
    compilation = _compile_action(ctx, source, deps, transitive_deps, attestations, presubmit, commit)
    docs = depset([compilation.docs] if compilation.docs else [])
    outputs = [compilation.module]

    return [
        DefaultInfo(files = depset(outputs)),
        OutputGroupInfo(
            docs = docs,
        ),
        ModuleVersionInfo(
            name = ctx.attr.module_name,
            version = ctx.attr.version,
            compatibility_level = ctx.attr.compatibility_level,
            bazel_compatibility = ctx.attr.bazel_compatibility,
            repo_name = ctx.attr.repo_name,
            deps = deps,
            transitive_deps = transitive_deps,
            source = source,
            attestations = attestations,
            presubmit = presubmit,
            commit = commit,
            module_bazel = ctx.file.module_bazel if ctx.file.module_bazel else None,
            build_bazel = ctx.file.build_bazel if ctx.file.build_bazel else None,
            proto = compilation.module,
            docs = docs,
        ),
    ]

module_version = rule(
    implementation = _module_version_impl,
    attrs = {
        "module_name": attr.string(mandatory = True),
        "version": attr.string(),
        "is_latest_version": attr.bool(),
        "compatibility_level": attr.int(default = 0),
        "bazel_compatibility": attr.string_list(),
        "repo_name": attr.string(),
        "deps": attr.label_list(providers = [ModuleDependencyInfo]),
        "mvs": attr.string_dict(),
        "dev_mvs": attr.string_dict(),
        "source": attr.label(providers = [ModuleSourceInfo]),
        "attestations": attr.label(providers = [ModuleAttestationsInfo]),
        "presubmit": attr.label(providers = [ModulePresubmitInfo]),
        "commit": attr.label(providers = [ModuleCommitInfo]),
        "build_bazel": attr.label(allow_single_file = True),
        "module_bazel": attr.label(allow_single_file = True),
        "_moduleversioncompiler": attr.label(
            default = "//cmd/moduleversioncompiler",
            executable = True,
            cfg = "exec",
        ),
        "_documentationcompiler": attr.label(
            default = "//cmd/documentationcompiler",
            executable = True,
            cfg = "exec",
        ),
        "_starlarkcompiler": attr.label(
            default = "//cmd/starlarkcompiler",
            executable = True,
            cfg = "exec",
        ),
        "_java_runtime": attr.label(
            default = "@bazel_tools//tools/jdk:current_java_runtime",
            cfg = "exec",
            providers = [java_common.JavaRuntimeInfo],
        ),
        "_starlarkserverjar": attr.label(
            default = "//cmd/starlarkcompiler:constellate_jar_file",
            allow_single_file = True,
        ),
        "_bazel_tools": attr.label(
            default = "@bazel_tools_starlark_library//tools:bzl_srcs",
            providers = [StarlarkLibraryFileInfo],
        ),
        # "_rules_cc_docs": attr.label(
        #     default = "@rules_cc_0.2.14_docs//:bundle",
        #     providers = [StarlarkLibraryFileInfo],
        # ),
    },
    provides = [ModuleVersionInfo],
)
