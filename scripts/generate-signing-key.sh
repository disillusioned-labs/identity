#!/usr/bin/env bash
# Generate (or rotate) the identity JWT signing key.
#
# Runs the cmd/generate-signing-key utility in a container built from the
# signing-key Dockerfile target, using whichever container engine is
# available: podman first, then docker. Override the choice with
# ENGINE=docker or ENGINE=podman.
#
# Usage:
#   scripts/generate-signing-key.sh            insert a new active key
#   scripts/generate-signing-key.sh --rotate   deactivate the current active
#                                              key in the same transaction
#
# The utility reads POSTGRES_DSN and AUTH_MASTER_KEY from .env.compose and
# joins the external docker network "data" to reach the containerised
# Postgres. A native Postgres on the host is not reachable from the container
# network - for that, run `go run ./cmd/generate-signing-key` with a .env that
# points at the host instead.
set -euo pipefail

cd "$(dirname "$0")/.."

if [ "$#" -gt 1 ]; then
	echo "usage: scripts/generate-signing-key.sh [--rotate]" >&2
	exit 2
fi

for arg in "$@"; do
	if [ "$arg" != "--rotate" ]; then
		echo "usage: scripts/generate-signing-key.sh [--rotate]" >&2
		exit 2
	fi
done

ENGINE="${ENGINE:-}"
if [ -z "$ENGINE" ]; then
	if command -v podman >/dev/null 2>&1; then
		ENGINE=podman
	elif command -v docker >/dev/null 2>&1; then
		ENGINE=docker
	else
		echo "error: neither podman nor docker found in PATH (set ENGINE to point at one)" >&2
		exit 1
	fi
fi

if [ ! -f .env.compose ]; then
	echo "error: .env.compose not found - copy .env.example and set POSTGRES_DSN + AUTH_MASTER_KEY first" >&2
	exit 1
fi

VERSION="$(git describe --tags --always --dirty 2>/dev/null || echo dev)"
COMMIT="$(git rev-parse --short HEAD 2>/dev/null || echo local)"
BUILD_DATE="$(date -u +%Y-%m-%dT%H:%M:%SZ)"

IMAGE=identity-signing-key:local

echo "building $IMAGE with $ENGINE ..."
"$ENGINE" build \
	--target signing-key \
	--build-arg "VERSION=$VERSION" \
	--build-arg "COMMIT=$COMMIT" \
	--build-arg "BUILD_DATE=$BUILD_DATE" \
	-t "$IMAGE" \
	.

echo "running signing-key utility on network 'data' ..."
exec "$ENGINE" run --rm \
	--network data \
	--env-file .env.compose \
	"$IMAGE" "$@"
