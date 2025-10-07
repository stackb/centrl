ModuleMaintainerInfo = provider(
    doc = "Info about a Module Maintainer",
    fields = {
        "email": "Maintainer email address",
        "username": "Maintainer name",
        "github": "GitHub username",
        "do_not_notify": "Whether to not notify this maintainer",
        "github_user_id": "GitHub user ID",
    },
)

ModuleMetadataInfo = provider(
    doc = "Info about a Module Metadata",
    fields = {
        "homepage": "Homepage URL for the module",
        "maintainers": "List of maintainer targets providing ModuleMaintainerInfo",
        "repository": "List of repository URLs",
        "versions": "List of version strings",
        "yanked_versions": "Dictionary mapping version to reason for yanking",
        "deprecated": "Deprecation message if deprecated",
    },
)

ModuleDependencyInfo = provider(
    doc = "Info about a Module Dependency",
    fields = {
        "name": "Dependency module name",
        "version": "Dependency module version",
        "dev": "Whether this is a dev dependency",
    },
)

ModuleVersionInfo = provider(
    doc = "Info about a Module Version",
    fields = {
        "name": "Module name",
        "version": "Module version",
        "compatibility_level": "Module compatibility level",
        "repo_name": "Repository name",
        "deps": "List of dependency targets providing ModuleDependencyInfo",
    },
)
