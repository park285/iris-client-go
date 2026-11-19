#!/usr/bin/env bash

release_asset_subjects() (
  set -eo pipefail
  cd -- "$1"
  sha256sum -- "$2" "$3" release-manifest.json SHA256SUMS \
    | jq -Rne '[inputs | capture("^(?<sha256>[0-9a-f]{64})  (?<name>.+)$") | {name, digest: {sha256}}] | select(length == 4) | sort'
)

verify_release_subjects() {
  jq -es --arg repo "$1" --arg tag "$2" --arg object "$3" --argjson assets "$4" '
    {uri: ("pkg:github/" + $repo + "@" + ($tag | @uri)), digest: {sha1: $object}} as $git |
    length == 1 and (.[0].verificationResult.statement.subject | sort) == ([$git] + $assets | sort)
  ' >/dev/null || {
    echo "release verification: signed subjects do not match the tag object and four assets" >&2
    return 1
  }
}

if [[ "${BASH_SOURCE[0]}" == "$0" ]]; then
  set -euo pipefail
  if (( $# != 4 )); then
    echo "usage: $0 <bundle-dir> <owner/repository> <tag> <commit> < verified-release.json" >&2
    exit 2
  fi
  SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
  . "${SCRIPT_DIR}/release-scope.sh"
  release_scope "$2" "$3" "$4"
  tag_object="$(git rev-parse --verify "refs/tags/$tag")"
  [[ "$(git rev-parse --verify "${tag_object}^{commit}")" == "$commit" ]] || {
    echo "release verification: tag target does not match requested commit" >&2
    exit 1
  }
  assets="$(release_asset_subjects "$1" "$archive" "$sbom")"
  verify_release_subjects "$repository" "$tag" "$tag_object" "$assets"
fi
