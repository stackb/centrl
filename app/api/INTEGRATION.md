# Integration with @rules_rs

This Rust worker is integrated into the Bazel workspace via `@rules_rs` and Cargo workspaces.

## How it works

1. **Root Cargo.toml** declares a workspace with `app/api` as a member
2. **@rules_rs** scans the workspace and generates `@crates//...` targets
3. **BUILD.bazel** references `@crates//:worker`, `@crates//:serde`, etc.
4. **Bazel** builds everything together

## Regenerating Cargo.lock

After adding new dependencies to `app/api/Cargo.toml`:

```bash
# Regenerate lock file
cargo generate-lockfile

# Update Bazel's view of dependencies
bazel run @crates//:vendor
```

## Building with Bazel

```bash
# Build the Rust library
bazel build //app/api:centrl_api_lib

# Build WASM for Cloudflare Worker
bazel build //app/api:centrl_api_wasm

# Deploy
bazel run //app/api:deploy
```

## Dependencies

All dependencies are managed in `app/api/Cargo.toml` and automatically available via `@crates//...` after running `cargo generate-lockfile`.

Current deps:
- `worker` - Cloudflare Workers SDK
- `serde` / `serde_json` - JSON serialization
- `prost` - Protocol Buffers
- `flate2` - Gzip decompression

## Proto Generation

The `build.rs` script generates Rust code from `//build/stack/bazel/bzlmod/v1:bcr.proto` at build time. This happens automatically when building with Cargo or Bazel.
