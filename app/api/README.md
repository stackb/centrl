# centrl-api

Rust-based Cloudflare Worker that provides a JSON API for the Bazel Central Registry.

## Architecture

The API serves registry data by:
1. Fetching `registry.pb.gz` from static assets (deployed alongside the SPA)
2. Decompressing and parsing the protobuf
3. Caching in memory for the worker's lifetime
4. Serving filtered/transformed JSON responses

## Routes

- `GET /api/modules` - List all modules
- `GET /api/modules/:name` - Get module details by name
- `GET /api/search?q=query` - Search modules
- `GET /api/registry` - Registry metadata

## Development

### Local Build

```bash
# Install worker-build
cargo install worker-build

# Build the worker
worker-build --release

# Run locally with wrangler
npx wrangler dev
```

### With Bazel

```bash
# Build the WASM module
bazel build //app/api:centrl_api_wasm

# Deploy to Cloudflare
bazel run //app/api:deploy
```

## Deployment

The worker is deployed to Cloudflare Workers and routes are configured to handle `/api/*` paths on centrl.io.

```bash
# Deploy to production
npx wrangler deploy --env production

# Deploy to staging
npx wrangler deploy --env staging
```

## Dependencies

- `worker` - Cloudflare Workers Rust SDK
- `prost` - Protocol Buffers for Rust
- `serde` / `serde_json` - JSON serialization
- `flate2` - Gzip decompression

## Proto Generation

The protobuf Rust code is generated from `//build/stack/bazel/bzlmod/v1:bcr.proto` at build time via the `build.rs` script.
