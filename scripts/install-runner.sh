#!/usr/bin/env bash
# Installs an orchestrator runner as a systemd service (RFC-109).
#
#   curl -fsSL https://orch.example.com/runner/install.sh | sudo bash -s -- \
#     --url https://orch.example.com --token <registration token> [--name box-1] [--slots 2] [--labels podman]
#
# Running it again upgrades the binary and the unit and keeps the registration.
set -euo pipefail

service_user=orchestrator
home_dir=/var/lib/orchestrator-runner
config=$home_dir/runner.json
bin=/usr/local/bin/orchestrator
unit_dir=/etc/systemd/system
unit=orchestrator-runner.service

url=""
token=${ORCHESTRATOR_RUNNER_TOKEN:-}
name=""
slots=2
labels=""

die() {
  printf 'install-runner: %s\n' "$*" >&2
  exit 1
}

say() {
  printf 'install-runner: %s\n' "$*"
}

usage() {
  cat <<'EOF'
usage: install-runner.sh --url <control plane> --token <registration token> [--name <name>] [--slots <1-16>] [--labels <a,b>]
EOF
}

while [ $# -gt 0 ]; do
  case $1 in
    --url | --token | --name | --slots | --labels)
      [ $# -ge 2 ] || die "$1 needs a value"
      case $1 in
        --url) url=$2 ;;
        --token) token=$2 ;;
        --name) name=$2 ;;
        --slots) slots=$2 ;;
        --labels) labels=$2 ;;
      esac
      shift 2
      ;;
    -h | --help)
      usage
      exit 0
      ;;
    *)
      usage >&2
      die "unknown argument: $1"
      ;;
  esac
done

[ "$(uname -s)" = Linux ] || die "the runner service needs Linux"
[ "$(id -u)" -eq 0 ] || die "run as root, for example through sudo"

url=${url%/}
[ -n "$url" ] || die "--url is required"
case $url in
  https://*) ;;
  http://127.0.0.1 | http://127.0.0.1:* | http://localhost | http://localhost:* | http://\[::1\] | http://\[::1\]:*) ;;
  *) die "--url must be https:// (http:// only on loopback)" ;;
esac
case $slots in
  '' | *[!0-9]*) die "--slots must be a number from 1 to 16" ;;
esac
if [ "$slots" -lt 1 ] || [ "$slots" -gt 16 ]; then
  die "--slots must be a number from 1 to 16"
fi

for tool in curl sha256sum git systemctl loginctl useradd runuser; do
  command -v "$tool" >/dev/null 2>&1 || die "$tool is not installed"
done
command -v podman >/dev/null 2>&1 || die "podman is not installed; the runner runs checks in rootless Podman"

case $(uname -m) in
  x86_64 | amd64) arch=amd64 ;;
  aarch64 | arm64) arch=arm64 ;;
  *) die "unsupported architecture $(uname -m)" ;;
esac
binary=orchestrator-linux-$arch

tmp=$(mktemp -d)
trap 'rm -rf "$tmp"' EXIT

fetch() {
  curl -fsSL --retry 3 -o "$tmp/$1" "$url/runner/download/$1" || die "could not download $1 from $url"
}

verify() {
  local file=$1 want got
  want=$(awk -v f="$file" '$2 == f || $2 == "*" f { print $1 }' "$tmp/SHA256SUMS")
  [ -n "$want" ] || die "$file is not listed in SHA256SUMS"
  got=$(sha256sum "$tmp/$file" | awk '{ print $1 }')
  [ "$want" = "$got" ] || die "$file does not match its SHA-256 in SHA256SUMS"
}

say "downloading $binary"
fetch SHA256SUMS
fetch "$binary"
fetch "$unit"
verify "$binary"
verify "$unit"

if ! id -u "$service_user" >/dev/null 2>&1; then
  say "creating the $service_user user"
  nologin=$(command -v nologin || echo /bin/false)
  useradd --create-home --home-dir "$home_dir" --shell "$nologin" --comment "Orchestrator runner" "$service_user"
fi
install -d -m 0700 -o "$service_user" -g "$service_user" "$home_dir"
uid=$(id -u "$service_user")

for ids in /etc/subuid /etc/subgid; do
  grep -q "^$service_user:" "$ids" 2>/dev/null ||
    die "$service_user has no range in $ids; add one, for example: usermod --add-subuids 200000-265535 --add-subgids 200000-265535 $service_user"
done

loginctl enable-linger "$service_user"
runtime=/run/user/$uid
for _ in $(seq 1 20); do
  [ -d "$runtime" ] && break
  sleep 0.5
done
[ -d "$runtime" ] || die "$runtime did not appear after enabling linger for $service_user"

as_user() {
  (cd "$home_dir" && runuser -u "$service_user" -- env HOME="$home_dir" XDG_RUNTIME_DIR="$runtime" "$@")
}

rootless=$(as_user podman info --format '{{.Host.Security.Rootless}}' 2>/dev/null) || die "podman info failed for $service_user"
[ "$rootless" = true ] || die "podman is not rootless for $service_user"

say "installing $bin"
install -m 0755 "$tmp/$binary" "$bin"
install -m 0644 "$tmp/$unit" "$unit_dir/$unit"
install -d -m 0755 "$unit_dir/$unit.d"
printf '[Service]\nEnvironment=XDG_RUNTIME_DIR=%s\n' "$runtime" >"$unit_dir/$unit.d/runtime.conf"

if [ -f "$config" ]; then
  say "keeping the registration in $config"
else
  [ -n "$token" ] || die "--token is required for a new runner"
  args=(runner register --url "$url" --slots "$slots" --config "$config" --data "$home_dir/work")
  [ -z "$name" ] || args+=(--name "$name")
  [ -z "$labels" ] || args+=(--labels "$labels")
  ORCHESTRATOR_RUNNER_TOKEN=$token as_user "$bin" "${args[@]}"
fi

systemctl daemon-reload
systemctl enable "$unit" >/dev/null
systemctl restart "$unit"
say "started $unit; follow it with: journalctl -u $unit -f"
