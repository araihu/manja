#!/usr/bin/env bash
set -euo pipefail

readonly expected_version="v0.21.11"

dagger_version() {
  dagger version | awk 'NR == 1 { print $2 }'
}

if [[ "${RUNNER_ENVIRONMENT:-}" == "self-hosted" ]] &&
  command -v dagger >/dev/null &&
  [[ "$(dagger_version)" == "$expected_version" ]]; then
  printf 'Verified Dagger CLI %s (%s)\n' "$expected_version" "$RUNNER_ENVIRONMENT"
  exit 0
fi

if [[ "${RUNNER_ENVIRONMENT:?RUNNER_ENVIRONMENT is required}" == "github-hosted" || "$RUNNER_ENVIRONMENT" == "self-hosted" ]]; then
  test "${RUNNER_OS:?RUNNER_OS is required}" = Linux
  case "${RUNNER_ARCH:?RUNNER_ARCH is required}" in
    X64)
      archive="dagger_v0.21.11_linux_amd64.tar.gz"
      expected_sha256="a2d3ef96e7305abc18663a3606d9c1c592278ba3c8f48e01d6be14eef72276ef"
      ;;
    ARM64)
      archive="dagger_v0.21.11_linux_arm64.tar.gz"
      expected_sha256="7a19fc16ce58d8aa80e273629a38a3cf3e49951df8928b38a908805c9a08cc50"
      ;;
    *)
      echo "Unsupported GitHub runner architecture: ${RUNNER_ARCH}" >&2
      exit 1
      ;;
  esac
  install_dir="${RUNNER_TEMP:?RUNNER_TEMP is required}/dagger-v0.21.11/bin"
  download="${RUNNER_TEMP}/dagger-v0.21.11/${archive}"
  mkdir -p "$install_dir" "$(dirname "$download")"
  curl --fail --silent --show-error --location \
    --output "$download" \
    "https://github.com/dagger/dagger/releases/download/v0.21.11/${archive}"
  printf '%s  %s\n' "$expected_sha256" "$download" | sha256sum --check --strict
  tar --extract --gzip --file "$download" --directory "$install_dir" dagger
  chmod 0755 "$install_dir/dagger"
  printf '%s\n' "$install_dir" >> "${GITHUB_PATH:?GITHUB_PATH is required}"
  PATH="$install_dir:$PATH"
elif [[ "$RUNNER_ENVIRONMENT" != "self-hosted" ]]; then
  echo "Unsupported runner environment: ${RUNNER_ENVIRONMENT}" >&2
  exit 1
fi

command -v dagger >/dev/null
resolved_version="$(dagger_version)"
if [[ "$resolved_version" != "$expected_version" ]]; then
  echo "Dagger CLI version mismatch: got ${resolved_version:-empty}, want $expected_version" >&2
  exit 1
fi
printf 'Verified Dagger CLI %s (%s)\n' "$resolved_version" "$RUNNER_ENVIRONMENT"
