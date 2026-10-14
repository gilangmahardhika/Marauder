#!/usr/bin/env bash
# Builds the runner release directory that ORCHESTRATOR_RUNNER_RELEASES points
# at (RFC-109): Linux binaries, the systemd unit, install-runner.sh, SHA256SUMS.
#
#   scripts/release.sh [version] [out dir]
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)
version=${1:-$(git -C "$root" describe --tags --always --dirty 2>/dev/null || echo dev)}
out=${2:-$root/dist/runner}

case $version in
  '' | *[!A-Za-z0-9._+-]*)
    printf 'release: version %q may use letters, digits, and . _ + - only\n' "$version" >&2
    exit 1
    ;;
esac

if command -v sha256sum >/dev/null 2>&1; then
  sum=(sha256sum)
else
  sum=(shasum -a 256)
fi

mkdir -p "$out"
for arch in amd64 arm64; do
  printf 'release: building orchestrator-linux-%s %s\n' "$arch" "$version"
  (
    cd "$root/backend"
    CGO_ENABLED=0 GOOS=linux GOARCH=$arch go build -trimpath \
      -ldflags "-s -w -X orchestrator/backend/internal/cli.Version=$version" \
      -o "$out/orchestrator-linux-$arch" ./cmd/orchestrator
  )
done
install -m 0644 "$root/deploy/systemd/orchestrator-runner.service" "$out/orchestrator-runner.service"
install -m 0644 "$root/scripts/install-runner.sh" "$out/install-runner.sh"
(
  cd "$out"
  "${sum[@]}" orchestrator-linux-amd64 orchestrator-linux-arm64 orchestrator-runner.service >SHA256SUMS
)
printf 'release: wrote %s; set ORCHESTRATOR_RUNNER_RELEASES=%s on the control plane\n' "$out" "$out"
