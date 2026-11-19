#!/usr/bin/env bash
set -euo pipefail

if (( $# != 4 )); then
  echo "usage: $0 <owner/repository> <tag> <commit> <output-dir>" >&2
  exit 2
fi

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
. "${SCRIPT_DIR}/release-scope.sh"
release_publish_scope "$1" "$2" "$3"
output_dir="$4"

release_source_identity
bash "$SCRIPT_DIR/check-release-module.sh" "$tag" "$commit"

if [[ -e "$output_dir" ]]; then
  echo "release bundle: output path already exists" >&2
  exit 1
fi

mkdir -m 0700 "$output_dir"
release_archive | gzip -n >"${output_dir}/${archive}"
chmod 0600 "${output_dir}/${archive}"

printf '{"schemaVersion":2,"repository":"%s","ref":"refs/tags/%s","tag":"%s","commit":"%s","module":"%s","moduleDirectory":"%s","version":"%s","artifactName":"%s","signerWorkflow":"%s/.github/workflows/release.yml","oidcIssuer":"https://token.actions.githubusercontent.com","sourceArchive":"%s","sbom":"%s"}\n' \
  "$repository" "$tag" "$tag" "$commit" "$module_path" "$module_dir" "$version" "$artifact_name" "$repository" "$archive" "$sbom" \
  >"${output_dir}/release-manifest.json"
chmod 0600 "${output_dir}/release-manifest.json"
