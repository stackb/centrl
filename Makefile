update_bcr:
	git submodule update --remote data/bazel-central-registry

clean_bcr:
	cd data/bazel-central-registry && git reset --hard && git clean -fd
