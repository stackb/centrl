bcr_init:
	git submodule update --init data/bazel-central-registry
	(cd data/bazel-central-registry && git sparse-checkout set --no-cone modules)
	
bcr_update:
	git submodule update --remote data/bazel-central-registry

bcr_clean:
	(cd data/bazel-central-registry && git reset --hard && git clean -fd)
	(cd data/bazel-central-registry && git sparse-checkout set --no-cone modules)

bcr:
	bazel run central_registry
