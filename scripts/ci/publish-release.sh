#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
# shellcheck source=scripts/ci/release-scope.sh
. "${SCRIPT_DIR}/release-scope.sh"
. "${SCRIPT_DIR}/verify-release-subjects.sh"

if (( $# != 4 )); then
  echo "usage: $0 <owner/repository> <tag> <commit> <bundle-dir>" >&2
  exit 2
fi

release_publish_scope "$1" "$2" "$3"
release_source_identity
bundle_dir="$4"

bash "$SCRIPT_DIR/verify-release-bundle.sh" "$bundle_dir" "$repository" "$tag" "$commit"
existing_tags="$(gh api --paginate "repos/${repository}/releases" --jq '.[].tag_name')"
if grep -Fxq -- "$tag" <<<"$existing_tags"; then
  echo "release publish: ${tag} already exists; refusing duplicate or rerun" >&2
  exit 1
fi

signer_args=(
  --repo "$repository"
  --signer-workflow "$repository/.github/workflows/release.yml"
  --cert-oidc-issuer https://token.actions.githubusercontent.com
  --source-ref "refs/tags/$tag"
  --source-digest "$commit"
  --deny-self-hosted-runners
)
for artifact in "$bundle_dir"/*; do
  gh attestation verify "$artifact" "${signer_args[@]}" --predicate-type https://slsa.dev/provenance/v1 >/dev/null
done
gh attestation verify "$bundle_dir/$archive" "${signer_args[@]}" --predicate-type https://spdx.dev/Document/v2.3 --format json \
  | jq -es --slurpfile doc "$bundle_dir/$sbom" 'length == 1 and any(.[0][]; .verificationResult.statement.predicate == $doc[0])' >/dev/null

latest_args=()
if [[ "$module_dir" != . ]]; then
  latest_args=(--latest=false)
fi

expected_assets="$(release_asset_subjects "$bundle_dir" "$archive" "$sbom")"
tag_object="$(git rev-parse --verify "refs/tags/$tag")"
verify_remote_tag() {
  local published_object
  published_object="$(gh api "repos/${repository}/git/ref/tags/${tag}" --jq '.object.sha')" || return 1
  [[ "$published_object" == "$tag_object" ]] || {
    echo "release publish: remote tag object does not match checkout" >&2; return 1;
  }
}
verify_remote_tag

gh release create "$tag" \
  --draft \
  "$bundle_dir/$archive" \
  "$bundle_dir/$sbom" \
  "$bundle_dir/release-manifest.json" \
  "$bundle_dir/SHA256SUMS" \
  --repo "$repository" \
  --verify-tag \
  --title "$tag" \
  --generate-notes

gh release view "$tag" --repo "$repository" --json assets,isDraft \
  | jq -es --argjson expected "$expected_assets" '
      length == 1 and (.[0] | .isDraft == true and
        ([.assets[] | {name, digest}] | sort) == ($expected | map({name, digest: ("sha256:" + .digest.sha256)}) | sort))
    ' >/dev/null
verify_remote_tag
gh release edit "$tag" --repo "$repository" --draft=false "${latest_args[@]}"

for attempt in 1 2 3 4 5; do
  if attestation="$(gh release verify "$tag" --repo "$repository" --format json)"; then
    verify_remote_tag
    verify_release_subjects "$repository" "$tag" "$tag_object" "$expected_assets" <<<"$attestation"
    exit 0
  fi
  if (( attempt < 5 )); then sleep 3; fi
done
echo "release publish: published release attestation could not be verified" >&2
exit 1
