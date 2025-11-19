"provides the serve_archive rule"

def _executable(ctx):
    ctx.actions.write(
        output = ctx.outputs.executable,
        content = """
set -e
if ! command -v serve &> /dev/null; then
    echo "Error: 'serve' command not found"
    echo "Please install it globally with: npm install -g serve"
    exit 1
fi
TMPDIR=$(mktemp -d)
trap "rm -rf $TMPDIR" EXIT
tar -xf {archive} -C $TMPDIR
ls -al $TMPDIR
serve -s $TMPDIR -S -p {port}
""".format(
            port = ctx.attr.port,
            archive = ctx.file.archive.short_path,
        ),
        is_executable = True,
    )

def _serve_archive_impl(ctx):
    _executable(ctx)

    return [
        DefaultInfo(
            executable = ctx.outputs.executable,
            files = depset([ctx.outputs.executable]),
            runfiles = ctx.runfiles(files = [ctx.file.archive]),
        ),
    ]

serve_archive = rule(
    implementation = _serve_archive_impl,
    attrs = {
        "archive": attr.label(
            allow_single_file = [".tar"],
        ),
        "port": attr.string(default = "8080"),
    },
    executable = True,
)
