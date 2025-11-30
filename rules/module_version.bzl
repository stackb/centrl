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

#     # print("module:", name, "(highest: %s)" % highest)
#     # for mv in versions.values():
#     #     print(" -", mv.version)

# moduleversion_by_key = {moduleversion.key: moduleversion for moduleversion in moduleversions}
# moduleversion_by_name = {moduleversion.name: moduleversion for moduleversion in moduleversions}
# module_names = sorted(moduleversion_by_name.keys())

# # here's what we need to do.  We need to have a list of files to extract docs for.  Foreach
# # of those files, we need to determine the files it expects to load.

# # foreach direct file to extract, process each load statement
# for moduleversion in moduleversions:
#     libpkg = moduleversion.starlarklibrary.label.package

#     for relative_filename, loads in moduleversion.starlarklibrary.loads.items():
#         for stmt in loads:
#             label = _parse_label(stmt)

#             # foreach load, we need to determine the specific moduleversion
#             # that will provide the file.
#             match = None  # moduleversion struct or None
#             if label.repo == "" or label.repo == moduleversion.name:
#                 match = moduleversion
#             if not match and label.repo == "bazel_tools":
#                 match = moduleversion_by_name["bazel_tools"]
#             if not match:
#                 dep = moduleversion.deps.get(label.repo, None)
#                 if dep:
#                     key = _make_moduleversion_key(dep.name, dep.version)
#                     match = moduleversion_by_key.get(key, None)
#             if not match:
#                 # fallback to name without version
#                 match = moduleversion_by_name.get(label.repo, None)

#                 # else:
#                 #     fail("in moduleversion %s: dep not found: %s (available: %r)" % (moduleversion.name, label.repo, moduleversion.deps.keys()))

#             if not match:
#                 print("unmapped label %s %s --> (%s not found)" % (moduleversion.key, stmt, label.repo))
#                 moduleversion.unknown_loads.append(label)

#             # else:
#             #     print("mapped label %s %s --> %s" % (moduleversion.key, stmt, match.key))

# # for mv in moduleversions:
# #     for dep in mv.deps.values():
# #         dep_key = _make_moduleversion_key(dep.name, dep.version)
# #         match = moduleversion_by_key.get(dep_key, None)
# #         if not match:
# #             fail("module dep: %s depends on %s, but that was not found" % (mv.key, dep_key))

# for key in sorted(moduleversion_by_key.keys()):
#     print("module key: %s (%s)" % (key, moduleversion_by_key[key].repo_name))
# for name in sorted(moduleversion_by_name.keys()):
#     print("module name: %s (%s)" % (name, moduleversion_by_name[name].key))

# # fail("WAIT")

# def _parse_label(lbl):
#     """Parse a Bazel label string into its components.

#     Args:
#         lbl: A label string like "@@repo//pkg:name" or "//pkg:name" or ":name"

#     Returns:
#         A struct with repo, pkg, and name fields
#     """
#     repo = ""
#     pkg = ""
#     name = ""

#     # Handle canonical repo name (@@repo)
#     if lbl.startswith("@@"):
#         repo_end = lbl.find("//", 2)
#         if repo_end != -1:
#             repo = lbl[2:repo_end]
#             lbl = lbl[repo_end:]
#         else:
#             # Just @@repo with no package
#             repo = lbl[2:]
#             return struct(repo = repo, pkg = "", name = "")
#     elif lbl.startswith("@"):
#         # Handle old-style @repo//pkg:name
#         repo_end = lbl.find("//", 1)
#         if repo_end != -1:
#             repo = lbl[1:repo_end]
#             lbl = lbl[repo_end:]
#         else:
#             # Just @repo with no package
#             repo = lbl[1:]
#             return struct(repo = repo, pkg = "", name = "")

#     # Handle package and name
#     if lbl.startswith("//"):
#         # //pkg:name or //pkg/subpkg:name
#         pkg_start = 2
#         colon_idx = lbl.find(":", pkg_start)
#         if colon_idx != -1:
#             pkg = lbl[pkg_start:colon_idx]
#             name = lbl[colon_idx + 1:]
#         else:
#             # //pkg with implicit name
#             pkg = lbl[pkg_start:]
#             name = pkg.split("/")[-1] if pkg else ""
#     elif lbl.startswith(":"):
#         # :name (relative to current package)
#         name = lbl[1:]
#     else:
#         # Bare name (relative to current package)
#         name = lbl

#     return struct(
#         repo = repo,
#         pkg = pkg,
#         name = name,
#     )

# def _sanitize_node_id(name, version):
#     """Sanitizes a module name and version to create a valid GraphViz node ID.

#     Args:
#         name: str - module name
#         version: str - module version (can be empty)

#     Returns:
#         str: A valid GraphViz node identifier
#     """

#     # Replace all special characters with underscores
#     safe_name = name.replace("-", "_").replace(".", "_").replace("/", "_").replace("+", "_")
#     if version:
#         safe_version = version.replace(".", "_").replace("-", "_").replace("/", "_").replace("+", "_")
#         return "%s_%s" % (safe_name, safe_version)
#     return safe_name

# def _generate_depgraph_dot(ctx, deps, transitive_deps):
#     """Generates a GraphViz dot file for the dependency graph.

#     Args:
#         ctx: the context object
#         deps: List<ModuleDependencyInfo> - direct dependencies
#         transitive_deps: DepSet<ModuleDependencyInfo> - all transitive dependencies

#     Returns:
#         struct with full and direct dot files
#     """

#     # Generate full transitive graph
#     full_dot = ctx.actions.declare_file(ctx.label.name + ".depgraph.dot")

#     # Generate simplified direct dependencies only graph
#     direct_dot = ctx.actions.declare_file(ctx.label.name + ".depgraph.direct.dot")

#     # Build the full transitive graph
#     lines = []
#     lines.append("digraph \"%s@%s\" {" % (ctx.attr.module_name, ctx.attr.version))
#     lines.append("  rankdir=TB;")  # Top-to-Bottom for better hierarchy
#     lines.append("  node [shape=box, style=rounded, fontname=\"Helvetica\", fontsize=11];")
#     lines.append("  edge [fontname=\"Helvetica\", fontsize=9, color=\"#666666\"];")
#     lines.append("  concentrate=true;")  # Merge multiple edges between same nodes
#     lines.append("  ranksep=0.8;")
#     lines.append("  nodesep=0.5;")
#     lines.append("")

#     # Add legend
#     lines.append("  subgraph cluster_legend {")
#     lines.append("    label=\"Legend\";")
#     lines.append("    fontsize=10;")
#     lines.append("    style=filled;")
#     lines.append("    color=lightgrey;")
#     lines.append("    node [shape=box];")
#     lines.append("    legend_root [label=\"Root Module\", fillcolor=lightblue, style=\"rounded,filled\"];")
#     lines.append("    legend_dev [label=\"Dev Dependency\", fillcolor=lightyellow, style=\"rounded,filled\"];")
#     lines.append("    legend_unresolved [label=\"Unresolved\", fillcolor=pink, style=\"rounded,filled\"];")
#     lines.append("    legend_cycle [label=\"Cycle\", fillcolor=orange, style=\"rounded,filled\"];")
#     lines.append("  }")
#     lines.append("")

#     # Add the root node
#     root_id = _sanitize_node_id(ctx.attr.module_name, ctx.attr.version)
#     lines.append("  \"%s\" [label=\"%s\\n@%s\", style=\"rounded,filled\", fillcolor=lightblue, fontsize=12, penwidth=2];" % (
#         root_id,
#         ctx.attr.module_name,
#         ctx.attr.version,
#     ))
#     lines.append("")

#     # Track all nodes and edges
#     nodes = {}  # node_id -> (name, version, dep)
#     edges = []  # (from_id, to_id, attributes)

#     # Add direct dependencies from root
#     for dep in deps:
#         dep_id = _sanitize_node_id(dep.name, dep.version)
#         nodes[dep_id] = (dep.name, dep.version, dep)
#         edge_attrs = ["penwidth=2"]  # Make direct edges thicker
#         if dep.dev:
#             edge_attrs.append("style=dashed")
#         if dep.unresolved:
#             edge_attrs.append("color=red")
#         edges.append((root_id, dep_id, ", ".join(edge_attrs)))

#     # Process all transitive dependencies
#     for dep in transitive_deps.to_list():
#         dep_id = _sanitize_node_id(dep.name, dep.version)
#         nodes[dep_id] = (dep.name, dep.version, dep)

#         # Add edges from this dep to its subdependencies
#         if hasattr(dep, "module") and dep.module and hasattr(dep.module, "deps"):
#             module_deps = dep.module.deps
#             deps_to_iterate = module_deps.values() if type(module_deps) == "dict" else module_deps

#             for subdep_item in deps_to_iterate:
#                 if hasattr(subdep_item, "name") and hasattr(subdep_item, "version"):
#                     subdep_id = _sanitize_node_id(subdep_item.name, subdep_item.version)
#                     edge_attrs = []
#                     if hasattr(subdep_item, "dev") and subdep_item.dev:
#                         edge_attrs.append("style=dashed")
#                     if hasattr(subdep_item, "unresolved") and subdep_item.unresolved:
#                         edge_attrs.append("color=red")
#                     edges.append((dep_id, subdep_id, ", ".join(edge_attrs)))

#     # Write all nodes
#     lines.append("  // Nodes (%d total)" % len(nodes))
#     for node_id, (name, version, dep) in nodes.items():
#         label = "%s\\n@%s" % (name, version) if version else name
#         attrs = ["label=\"%s\"" % label]

#         if hasattr(dep, "unresolved") and dep.unresolved:
#             attrs.append("fillcolor=pink")
#             attrs.append("style=\"rounded,filled\"")
#         elif hasattr(dep, "dev") and dep.dev:
#             attrs.append("fillcolor=lightyellow")
#             attrs.append("style=\"rounded,filled\"")
#         elif hasattr(dep, "cycle") and dep.cycle:
#             attrs.append("fillcolor=orange")
#             attrs.append("style=\"rounded,filled\"")

#         lines.append("  \"%s\" [%s];" % (node_id, ", ".join(attrs)))

#     lines.append("")
#     lines.append("  // Edges (%d total)" % len(edges))

#     # Write all edges
#     for from_id, to_id, attrs in edges:
#         if attrs:
#             lines.append("  \"%s\" -> \"%s\" [%s];" % (from_id, to_id, attrs))
#         else:
#             lines.append("  \"%s\" -> \"%s\";" % (from_id, to_id))

#     lines.append("}")

#     ctx.actions.write(
#         output = full_dot,
#         content = "\n".join(lines) + "\n",
#     )

#     # Generate simplified direct dependencies only graph
#     direct_lines = []
#     direct_lines.append("digraph \"%s@%s (Direct Dependencies)\" {" % (ctx.attr.module_name, ctx.attr.version))
#     direct_lines.append("  rankdir=TB;")
#     direct_lines.append("  node [shape=box, style=\"rounded,filled\", fontname=\"Helvetica\", fontsize=12];")
#     direct_lines.append("  edge [fontname=\"Helvetica\", penwidth=2];")
#     direct_lines.append("")

#     # Root node
#     direct_lines.append("  \"%s\" [label=\"%s\\n@%s\", fillcolor=lightblue, fontsize=14, penwidth=3];" % (
#         root_id,
#         ctx.attr.module_name,
#         ctx.attr.version,
#     ))
#     direct_lines.append("")

#     # Only direct dependencies
#     for dep in deps:
#         dep_id = _sanitize_node_id(dep.name, dep.version)
#         label = "%s\\n@%s" % (dep.name, dep.version) if dep.version else dep.name

#         node_attrs = ["label=\"%s\"" % label]
#         if hasattr(dep, "unresolved") and dep.unresolved:
#             node_attrs.append("fillcolor=pink")
#         elif hasattr(dep, "dev") and dep.dev:
#             node_attrs.append("fillcolor=lightyellow")
#         else:
#             node_attrs.append("fillcolor=white")

#         direct_lines.append("  \"%s\" [%s];" % (dep_id, ", ".join(node_attrs)))

#         # Edge attributes
#         edge_attrs = []
#         if dep.dev:
#             edge_attrs.append("style=dashed")
#         if dep.unresolved:
#             edge_attrs.append("color=red")

#         if edge_attrs:
#             direct_lines.append("  \"%s\" -> \"%s\" [%s];" % (root_id, dep_id, ", ".join(edge_attrs)))
#         else:
#             direct_lines.append("  \"%s\" -> \"%s\";" % (root_id, dep_id))

#     direct_lines.append("")
#     direct_lines.append("  // Total: %d direct dependencies" % len(deps))
#     direct_lines.append("}")

#     ctx.actions.write(
#         output = direct_dot,
#         content = "\n".join(direct_lines) + "\n",
#     )

#     return struct(full = full_dot, direct = direct_dot)

# def _print_dependency_tree(transitive_deps, module_name, module_version):
#     """Prints a formatted dependency tree from transitive_deps.

#     Args:
#         transitive_deps: DepSet<ModuleDependencyInfo> - all transitive dependencies
#         module_name: str - name of the root module
#         module_version: str - version of the root module
#     """
#     deps_list = transitive_deps.to_list()

#     print("=" * 80)
#     print("Dependency Tree for %s@%s" % (module_name, module_version))
#     print("=" * 80)
#     print("Total transitive dependencies: %d" % len(deps_list))
#     print("")

#     # Group by direct vs transitive
#     direct_deps = []
#     all_deps_by_name = {}

#     for dep in deps_list:
#         key = "%s@%s" % (dep.name, dep.version) if dep.version else dep.name
#         all_deps_by_name[key] = dep

#     # Print dependency tree
#     print("Dependencies:")
#     for dep in deps_list:
#         version_str = "@%s" % dep.version if dep.version else ""
#         repo_str = " (repo: %s)" % dep.repo_name if dep.repo_name and dep.repo_name != dep.name else ""
#         dev_str = " [dev]" % () if dep.dev else ""
#         unresolved_str = " [UNRESOLVED]" % () if dep.unresolved else ""
#         cycle_str = " [CYCLE]" % () if dep.cycle else ""

#         print("  - %s%s%s%s%s%s" % (dep.name, version_str, repo_str, dev_str, unresolved_str, cycle_str))

#         # Print subdependencies if available
#         if hasattr(dep, "module") and dep.module:
#             module_version = dep.module
#             if hasattr(module_version, "deps") and module_version.deps:
#                 # deps can be either a dict or a list
#                 deps_to_iterate = []
#                 if type(module_version.deps) == "dict":
#                     # dict mapping repo_name -> ModuleDependencyInfo
#                     deps_to_iterate = module_version.deps.values()
#                 else:
#                     # assume it's a list
#                     deps_to_iterate = module_version.deps

#                 for subdep_item in deps_to_iterate:
#                     # subdep_item could be a target or already a ModuleDependencyInfo
#                     # Try to extract it as a provider first, otherwise use directly
#                     subdep_info = None
#                     if hasattr(subdep_item, "name") and hasattr(subdep_item, "version"):
#                         # It's already a ModuleDependencyInfo or similar struct
#                         subdep_info = subdep_item

#                     if subdep_info:
#                         subdep_version_str = "@%s" % subdep_info.version if subdep_info.version else ""
#                         print("      -> %s%s" % (subdep_info.name, subdep_version_str))

#     print("=" * 80)

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
        repo_name = starlarklibrary.repo_name,
        canonical_name = starlarklibrary.repo_name,
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
    # args.add("--bazel_tools_repo_name", ctx.attr._bazel_tools_docs.label.repo_name)

    for mv in mvs:
        for file in mv.starlarklibrary.srcs:
            args.add("--bzl_file=%s:%s" % (mv.name, file.path))

    for mv in mvs:
        for dep in mv.deps.values():
            args.add("--module_dep=%s:%s=%s" % (mv.name, dep.name, dep.repo_name))

    # for dep in deps:
    #     args.add("--module_dep=%s=%s:%s:%s" % (source.starlarklibrary.repo_name, dep.name, dep.repo_name, dep.version))
    # for file in source.starlarklibrary.srcs:
    #     args.add("--bzl_file=%s=%s" % (source.starlarklibrary.repo_name, file.path))

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
