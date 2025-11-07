"""html_template.bzl provides the html_template rule.
"""

HtmlTemplateInfo = provider("provider that carries the index file", fields = {
    "files": "List[File]: list of source files",
})

def _render(ctx, files, output_file):
    args = ctx.actions.args()
    args.use_param_file("@%s", use_always = True)
    args.set_param_file_format("multiline")

    args.add("--rule_kind=%s" % "html_template")
    args.add("--rule_label=%s" % str(ctx.label))
    args.add("--output_file", output_file)
    args.add_all(files)

    ctx.actions.run(
        mnemonic = "RenderHTML",
        progress_message = "Rendering %s (%d files)" % (str(ctx.label), len(files)),
        execution_requirements = {
            "supports-workers": "1",
            "requires-worker-protocol": "proto",
        },
        executable = ctx.executable._html_renderer,
        arguments = [args],
        inputs = files,
        outputs = [output_file],
    )

    return output_file

def _html_template_impl(ctx):
    _render(ctx, ctx.files.srcs, ctx.outputs.pb)
    _render(ctx, ctx.files.srcs, ctx.outputs.json)

    return [
        DefaultInfo(
            files = depset([ctx.outputs.pb]),
        ),
        OutputGroupInfo(
            json = depset([ctx.outputs.json]),
        ),
        HtmlTemplateInfo(
            index = ctx.outputs.pb,
            files = ctx.files.srcs,
        ),
    ]

html_template = rule(
    implementation = _html_template_impl,
    attrs = {
        "srcs": attr.label_list(
            doc = "list of srcs to be rendered",
            allow_files = [".pb"],
        ),
        "deps": attr.label_list(
            doc = "list of child html_template rules to be merged",
            providers = [HtmlTemplateInfo],
        ),
        "_html_renderer": attr.label(
            default = Label("//cmd/html_renderer"),
            cfg = "exec",
            executable = True,
            doc = "the parser tool",
        ),
    },
    outputs = {
        "pb": "%{name}.pb",
        "json": "%{name}.json",
    },
)
