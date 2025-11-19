"provides the repository_metadata rule"

load("//rules:providers.bzl", "RepositoryMetadataInfo")

def _repository_metadata_write_action(ctx):
    output = ctx.actions.declare_file(ctx.label.name + ".json")

    data = struct(
        type = ctx.attr.type,
        organization = ctx.attr.organization,
        name = ctx.attr.repo_name,
        description = ctx.attr.description,
        stargazers = ctx.attr.stargazers,
        languages = ctx.attr.languages,
        canonical_name = ctx.attr.canonical_name,
        primary_language = ctx.attr.primary_language,
    )

    ctx.actions.write(output, json.encode(data))

    return output

def _repository_metadata_impl(ctx):
    json_file = _repository_metadata_write_action(ctx)

    return [
        DefaultInfo(files = depset([json_file])),
        RepositoryMetadataInfo(
            type = ctx.attr.type,
            json_file = json_file,
            organization = ctx.attr.organization,
            canonical_name = ctx.attr.canonical_name,
            repo_name = ctx.attr.repo_name,
            description = ctx.attr.description,
            stargazers = ctx.attr.stargazers,
            languages = ctx.attr.languages,
            primary_language = ctx.attr.primary_language,
        ),
    ]

repository_metadata = rule(
    implementation = _repository_metadata_impl,
    attrs = {
        "type": attr.string(
            doc = "Repository type (e.g., 'GITHUB', 'REPOSITORY_TYPE_UNKNOWN')",
        ),
        "organization": attr.string(
            doc = "Organization or owner name",
        ),
        "canonical_name": attr.string(
            doc = "The canonical repository name",
        ),
        "repo_name": attr.string(
            doc = "Repository name",
        ),
        "description": attr.string(
            doc = "Repository description",
        ),
        "stargazers": attr.int(
            doc = "Number of stargazers",
        ),
        "languages": attr.string_dict(
            doc = "Map of programming languages to line counts",
        ),
        "primary_language": attr.string(
            doc = "Name of the language having the most line counts",
        ),
    },
    provides = [RepositoryMetadataInfo],
)
