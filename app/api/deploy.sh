#!/bin/bash
set -euo pipefail

# Deploy script for Cloudflare Worker
# Usage: bazel run //app/api:deploy [-- --env staging]

cd "$(dirname "$0")"

echo "Building worker with cargo..."
cargo install -q worker-build
worker-build --release

echo "Deploying to Cloudflare..."
npx wrangler deploy "$@"

echo "✅ Deployment complete!"
