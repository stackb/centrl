"provides the module_registry rule"

load("@build_stack_rules_proto//rules:starlark_library.bzl", "StarlarkLibraryFileInfo")
load(
    "//rules:providers.bzl",
    "ModuleDependencyCycleInfo",
    "ModuleMetadataInfo",
    "ModuleRegistryInfo",
)

# buildifier: disable=name-conventions // for case sensitive search of other var
# name in .go file(s)
starlarkRepositoryPartitionKey = "--bzl_srcs--"

def _write_repos_json_action(ctx, deps):
    output = ctx.actions.declare_file(ctx.label.name + ".repos.json")

    repos = []
    for dep in deps:
        repos.extend(dep.repository)

    ctx.actions.write(output, json.encode(depset(repos).to_list()))

    return output

def _write_registry_languages_json_action(ctx, mds):
    output = ctx.actions.declare_file(ctx.label.name + ".languages.json")

    # Merge all language dictionaries
    languages = {}
    for md in mds:
        if getattr(md, "languages"):
            languages.update(md.languages)

    # Extract language names as a sorted list
    language_list = sorted(languages.keys())

    ctx.actions.write(output, json.encode(language_list))

    return output

def _compile_codesearch_index_action(ctx, deps):
    output = ctx.actions.declare_file("csearchindex")
    files = []

    for module in deps:
        files.append(module.build_bazel)
        for mv in module.deps:
            files.append(mv.module_bazel)
            files.append(mv.build_bazel)
            # files.append(mv.source.source_json)

    args = ctx.actions.args()
    args.add("--output_file")
    args.add(output)
    args.add_all(files)

    ctx.actions.run(
        executable = ctx.executable._codesearchcompiler,
        arguments = [args],
        inputs = files,
        outputs = [output],
        mnemonic = "CompileCodesearchIndex",
    )

    return output

def _get_module_id_from_bzl_repository_repo_name(repo_name):
    parts = repo_name.split("+")
    if len(parts) == 0:
        return repo_name
    last_part = parts[len(parts) - 1]
    name_version = last_part[len("bzl."):]
    return name_version.replace("---", "@")

def _add_args_for_module(args, module_name, srcs, deps):
    for file in srcs:
        args.add("--bzl_file=%s:%s" % (module_name, file.path))
    if len(deps) == 0:
        args.add("--module_dep=%s:NONE" % module_name)
    else:
        for dep in deps:
            args.add("--module_dep=%s:%s=%s" % (module_name, dep.name, dep.repo_name))

def _compile_docs_action_for_module_version(ctx, mv, versions_by_id):
    """Generate documentation extraction action for a single module version.

    Args:
        ctx: The rule context
        mv: ModuleVersionInfo provider for the module version
        versions_by_id: dict k->v where k is string like "rules_cc@0.0.9" and v is the ModuleVersionInfo provider
    Returns:
        Output file for the documentation, or None if cannot generate
    """

    # Declare output file for this module version
    output = ctx.actions.declare_file("%s/%s/documentationinfo.pb" % (mv.name, mv.version))

    # JavaRuntimeInfo
    java_runtime = ctx.attr._java_runtime[java_common.JavaRuntimeInfo]
    java_executable = java_runtime.java_executable_exec_path

    # StarlarkLibraryFileInfo
    bazel_tools_lib = ctx.attr._bazel_tools[StarlarkLibraryFileInfo]

    # List[StarlarkLibraryFileInfo]
    direct_libs = [bazel_tools_lib, mv.bzl_srcs] + mv.bzl_deps

    # DepSet[StarlarkLibraryFileInfo]
    transitive_libs = depset(direct_libs, transitive = [lib.transitive_deps for lib in direct_libs])

    # List[DepSet]
    transitive_srcs = [depset(lib.srcs) for lib in transitive_libs.to_list()]

    # DepSet[File]
    inputs = depset([ctx.file._starlarkserverjar], transitive = transitive_srcs)

    # Build arguments
    args = ctx.actions.args()
    args.use_param_file("@%s", use_always = True)
    args.set_param_file_format("multiline")

    args.add("--output_file", output)

    args.add("--port", 3679)  # e.g. java -jar ./cmd/starlarkcompiler/constellate.jar --listen_port=3679
    args.add("--java_interpreter_file", java_executable)
    args.add("--server_jar_file", ctx.file._starlarkserverjar)
    args.add("--workspace_cwd", ctx.bin_dir.path)
    args.add("--workspace_output_base", "/private/var/tmp/_bazel_pcj/4d50590a9155e202dda3b0ac2e024c3f")

    # Add bzl_files and module_deps without flattening depsets

    # 1. Bazel tools
    _add_args_for_module(args, "bazel_tools", bazel_tools_lib.srcs, [])

    # 2. Root module (bzl_srcs)
    _add_args_for_module(args, mv.name, mv.bzl_srcs.srcs, mv.deps)

    # 3. Dependencies (bzl_deps)
    for bzl_dep in mv.bzl_deps:
        id = _get_module_id_from_bzl_repository_repo_name(bzl_dep.label.repo_name)
        dep_mv = versions_by_id.get(id)

        if not dep_mv:
            # buildifier: disable=print
            print("🔴 WARN for module %s, the module for bzl source dependency %s was not found!" % (dep_mv.id, bzl_dep.label.repo_name))
            continue

        # buildifier: disable=print
        # print("🟢 module %s, the module for bzl source dependency is %s " % (dep_mv.id, bzl_dep.label.repo_name))

        _add_args_for_module(args, dep_mv.name, bzl_dep.srcs, dep_mv.deps)

    # Add root module source files as positional arguments
    args.add_all(mv.bzl_srcs.srcs)

    # Run the action
    ctx.actions.run(
        mnemonic = "CompileModuleInfo",
        progress_message = "Extracting docs for %s@%s (%d files)" % (mv.name, mv.version, len(mv.bzl_srcs.srcs)),
        execution_requirements = {
            "supports-workers": "0",
            "requires-worker-protocol": "proto",
        },
        executable = ctx.executable._starlarkcompiler,
        arguments = [args],
        inputs = inputs,
        outputs = [output],
        tools = java_runtime.files.to_list(),
    )

    return output

def _compile_docs_actions_for_module(ctx, module, versions_by_id):
    outputs = []
    for mv in module.deps:
        if not mv.is_latest_version:
            # print("skip %s %s (not latest): " % (mv.name, mv.version))
            continue
        if not mv.bzl_srcs:
            # print("skip %s %s (no bzl_srcs): " % (mv.name, mv.version))
            continue
        if not mv.bzl_srcs.srcs:
            # print("skip %s %s (no bzl_srcs): " % (mv.name, mv.version))
            continue

        # if not module.name == "rules_oci":
        #     continue
        outputs.append(_compile_docs_action_for_module_version(ctx, mv, versions_by_id))
    return outputs

def _compile_docs_actions(ctx, deps):
    versions_by_id = {}
    for m in deps:
        for v in m.deps:
            versions_by_id[v.id] = v

    outputs = []
    for module in deps:
        outputs.extend(_compile_docs_actions_for_module(ctx, module, versions_by_id))
    return outputs

def _compile_colors_action(ctx, colors_json, languages_json):
    output = ctx.actions.declare_file(ctx.label.name + ".colors.css")

    # Build arguments for the compiler
    args = ctx.actions.args()
    args.add("--output_file")
    args.add(output)
    args.add("--colors_json_file")
    args.add(colors_json)
    args.add("--languages_json_file")
    args.add(languages_json)

    ctx.actions.run(
        executable = ctx.executable._colorcompiler,
        arguments = [args],
        inputs = [colors_json, languages_json],
        outputs = [output],
        mnemonic = "CompileColors",
        progress_message = "Compiling css colors for languages",
    )

    return output

def _compile_sitemap_action(ctx, registry_pb):
    output = ctx.actions.declare_file("sitemap.xml")

    # Build arguments for the compiler
    args = ctx.actions.args()
    args.add("--output_file")
    args.add(output)
    args.add("--registry_file")
    args.add(registry_pb)
    args.add("--base_url")
    args.add(ctx.attr.registry_url)

    ctx.actions.run(
        executable = ctx.executable._sitemapcompiler,
        arguments = [args],
        inputs = [registry_pb],
        outputs = [output],
        mnemonic = "CompileSitemap",
        progress_message = "Compiling sitemap",
    )

    return output

def _write_robots_txt_action(ctx):
    output = ctx.actions.declare_file("robots.txt")

    ctx.actions.write(output, """User-agent: *
Allow: /
Disallow: /settings
Sitemap: {registry_url}/sitemap.xml
""".format(
        registry_url = ctx.attr.registry_url,
    ))

    return output

def _compile_action(ctx, modules):
    output = ctx.actions.declare_file("registry.pb")

    args = ctx.actions.args()
    args.add("--output_file")
    args.add(output)
    args.add("--registry_url")
    args.add(ctx.attr.registry_url)
    if ctx.attr.repository_url:
        args.add("--repository_url")
        args.add(ctx.attr.repository_url)
    if ctx.attr.branch:
        args.add("--branch")
        args.add(ctx.attr.branch)
    if ctx.attr.commit:
        args.add("--commit")
        args.add(ctx.attr.commit)
    if ctx.attr.commit_date:
        args.add("--commit_date")
        args.add(ctx.attr.commit_date)
    args.add_all(modules)

    ctx.actions.run(
        executable = ctx.executable._compiler,
        arguments = [args],
        inputs = modules,
        outputs = [output],
        mnemonic = "CompileRegistry",
        progress_message = "Compiling registry for %{label}",
    )

    return output

def _module_registry_impl(ctx):
    deps = [d[ModuleMetadataInfo] for d in ctx.attr.deps]
    cycles = [d[ModuleDependencyCycleInfo] for d in ctx.attr.cycles]
    modules = [d.proto for d in deps]
    repository_metadatas = [dep.repository_metadata for dep in deps if dep.repository_metadata]

    registry_pb = _compile_action(ctx, modules)
    repos_json = _write_repos_json_action(ctx, deps)
    languages_json = _write_registry_languages_json_action(ctx, repository_metadatas)
    colors_css = _compile_colors_action(ctx, ctx.file._colors_json, languages_json)
    sitemap_xml = _compile_sitemap_action(ctx, registry_pb)
    robots_txt = _write_robots_txt_action(ctx)
    codesearch_index = _compile_codesearch_index_action(ctx, deps)
    docs = _compile_docs_actions(ctx, deps)

    return [
        DefaultInfo(files = depset([registry_pb])),
        OutputGroupInfo(
            repos_json = [repos_json],
            languages_json = [languages_json],
            colors_css = [colors_css],
            sitemap_xml = [sitemap_xml],
            robots_txt = [robots_txt],
            registry_pb = [registry_pb],
            codesearch_index = [codesearch_index],
            docs = depset(docs),
        ),
        ModuleRegistryInfo(
            deps = depset(deps),
            cycles = depset(cycles),
            proto = registry_pb,
            repository_url = ctx.attr.repository_url,
            registry_url = ctx.attr.registry_url,
            branch = ctx.attr.branch,
            commit = ctx.attr.commit,
            commit_date = ctx.attr.commit_date,
        ),
    ]

module_registry = rule(
    implementation = _module_registry_impl,
    attrs = {
        "deps": attr.label_list(providers = [ModuleMetadataInfo]),
        "cycles": attr.label_list(providers = [ModuleDependencyCycleInfo]),
        "repository_url": attr.string(doc = "Repository URL of the registry (e.g. 'https://github.com/bazelbuild/bazel-central-registry')"),
        "registry_url": attr.string(doc = "URL of the registry UI (e.g. 'https://registry.bazel.build')", mandatory = True),
        "branch": attr.string(doc = "Branch name of the repository data (e.g. 'main')"),
        "commit": attr.string(doc = "Commit sha1 of the repository data"),
        "commit_date": attr.string(doc = "Timestamp of the commit date (same format as: git log --format='%ci')"),
        "_colors_json": attr.label(
            default = "@com_github_ozh_github_colors//:colors_json",
            allow_single_file = True,
        ),
        "_compiler": attr.label(
            default = "//cmd/registrycompiler",
            executable = True,
            cfg = "exec",
        ),
        "_colorcompiler": attr.label(
            default = "//cmd/colorcompiler",
            executable = True,
            cfg = "exec",
        ),
        "_sitemapcompiler": attr.label(
            default = "//cmd/sitemapcompiler",
            executable = True,
            cfg = "exec",
        ),
        "_codesearchcompiler": attr.label(
            default = "//cmd/codesearchcompiler",
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
            default = "@bzl.bazel_tools//tools:bzl_srcs",
            providers = [StarlarkLibraryFileInfo],
        ),
    },
    provides = [ModuleRegistryInfo],
)
