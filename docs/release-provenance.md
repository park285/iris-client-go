# Release provenance

Future releases use GitHub Actions OIDC and Sigstore keyless certificates as the signing authority.
No workstation or long-lived organization signing key is copied into CI. Existing tags and releases
are not moved, recreated, or retroactively signed.

The tag workflow publishes the independent adapter. Verification also supports existing SDK releases:

| Tag | Committed Go module | Source directory | Asset prefix |
| --- | --- | --- | --- |
| `v6.MINOR.PATCH` | `github.com/park285/iris-client-go/v6` | `.` | `iris-client-go-v6.MINOR.PATCH` |
| `v5.MINOR.PATCH` (existing releases, verification only) | `github.com/park285/iris-client-go/v5` | `.` | `iris-client-go-v5.MINOR.PATCH` |
| `adapters/valkeydedup/v1.MINOR.PATCH` | `github.com/park285/iris-client-go/adapters/valkeydedup` | `adapters/valkeydedup` | `iris-client-go-valkeydedup-v1.MINOR.PATCH` |

The root module is published as `v6.*`. Do not tag a new root `v5.*` from this tree: it removes the
public `/v5/valkeydedup` import. Existing `v5.*` tags stay valid for consumers that have not moved to
`/v6`, and publishing the adapter does not change them.

Minor and patch numbers are decimal integers without leading zeros; prerelease and build suffixes
are rejected. The gate runs only for the selected module, with `GOWORK=off`, its published
dependencies, and no `replace`. The tag must resolve to the checked-out commit, already contained
in `origin/main`. Only the publish job gets `contents: write`, `id-token: write`, and
`attestations: write`; PR, CI and security workflows cannot mint release attestations.

The root archive excludes `adapters/valkeydedup`. The adapter archive contains only that directory
under its asset prefix, preserving its repository-relative path. Both include their own `go.mod`,
`go.sum`, and `LICENSE`. Syft scans the extracted archive, not the whole checkout. The schema-2
manifest binds repository, tag/ref, commit, module path, source directory, version, asset names,
signer workflow and OIDC issuer. Verification requires exactly four files, checks `SHA256SUMS` and
SPDX document identity, and compares the uncompressed archive byte-for-byte to the selected Git tree.
The offline verifier accepts schema 1 only for the exact root tag and commit pairs recorded in
`scripts/ci/schema-1-releases.txt`. These are the published immutable manifests, not every tag or
ancestor of `v5.2.1`. This list is frozen; new bundles use schema 2.

### Reproduce a bundle without publishing

Run from the SDK repository root with Git, Go from the selected `go.mod`, the repository's pinned
CPython (`.python-version`, resolved offline with `uv`), Bash, tar, gzip, GNU coreutils, and Syft
v1.57.0. Use an existing trusted tag and full history including `refs/remotes/origin/main`, with
HEAD at the tag target. Set `GITHUB_REF` and `GITHUB_SHA` to that identity or leave them unset.
An uncommitted working tree or developer workspace cannot stand in for the tagged source.

```bash
set -euo pipefail
export GOWORK=off
repo=park285/iris-client-go
tag=adapters/valkeydedup/v1.0.0
commit="$(git rev-parse --verify "refs/tags/${tag}^{commit}")"
source scripts/ci/release-scope.sh
release_scope "$repo" "$tag" "$commit"
tmp="$(mktemp -d)"
bundle="$tmp/dist"
bash scripts/ci/prepare-release-bundle.sh "$repo" "$tag" "$commit" "$bundle"
mkdir -m 0700 "$tmp/source"
tar -xzf "$bundle/$archive" -C "$tmp/source"
syft scan "dir:$tmp/source" -o "spdx-json=$bundle/$sbom"
bash scripts/ci/finalize-release-bundle.sh "$bundle" "$repo" "$tag" "$commit"
bash scripts/ci/verify-release-bundle.sh "$bundle" "$repo" "$tag" "$commit"
```

Finalization already runs verification; the last command can also be run independently on a
download. Neither bundle construction nor its checks are proof that a Go version is published.

### Publication and consumer verification

Publication requires separate authorization, GitHub-hosted Actions, GitHub release immutability,
GitHub CLI with attestation verification, and the job permissions above. Before authorizing a
publication, the release owner checks that `GET /repos/park285/iris-client-go/immutable-releases`
returns `enabled: true`, that release tag namespaces are protected, and that every new
release tag targets a commit with this release workflow. Old commits run their old workflows,
even when a new tag uses the current major. Retired tag namespaces must not accept new tags. The job
does not receive repository administration access. These rules and a trusted checkout are
prerequisites; checking a tag object before and after publication is not an atomic lock.
The workflow attests all four assets and separately binds the SPDX SBOM to the archive. Before
creating a release it verifies repository, signer workflow, OIDC issuer, tag ref, commit SHA,
predicate and hosted runner. An existing release or failed lookup stops publication. Adapter
releases use `--latest=false`. The workflow compares all draft asset names and SHA-256 digests
before publishing. Publisher and consumer use `verify-release-subjects.sh` to compare the entire
signed subject list, regardless of order: one GitHub package URI for the repository and tag with
the tag object's SHA-1, and one `(filename, SHA-256)` for each of the four local files. Missing,
duplicate or unexpected subjects fail verification. A failed check may leave a draft or published
release: inspect its state; do not recreate a tag or automatically retry a write.

Verify a downloaded release from a trusted checkout of the current verification scripts and
the release tags, not by running scripts supplied in the downloaded archive:

```bash
set -euo pipefail
export GOWORK=off
tag=adapters/valkeydedup/v1.0.0
repo=park285/iris-client-go
commit="$(git rev-parse --verify "refs/tags/${tag}^{commit}")"
source scripts/ci/release-scope.sh
release_scope "$repo" "$tag" "$commit"
bundle="$(mktemp -d)"
gh release download "$tag" --repo "$repo" --dir "$bundle"
bash scripts/ci/verify-release-bundle.sh "$bundle" "$repo" "$tag" "$commit"
signer=(
  --repo "$repo" --signer-workflow "$repo/.github/workflows/release.yml"
  --cert-oidc-issuer https://token.actions.githubusercontent.com
  --source-ref "refs/tags/$tag" --source-digest "$commit" --deny-self-hosted-runners
)
gh release verify "$tag" --repo "$repo" --format json \
  | bash scripts/ci/verify-release-subjects.sh "$bundle" "$repo" "$tag" "$commit"
for asset in "$bundle"/*; do
  gh attestation verify "$asset" "${signer[@]}" --predicate-type https://slsa.dev/provenance/v1
  gh release verify-asset "$tag" "$asset" --repo "$repo"
done
gh attestation verify "$bundle/$archive" "${signer[@]}" \
  --predicate-type https://spdx.dev/Document/v2.3 --format json \
  | jq -es --slurpfile doc "$bundle/$sbom" 'length == 1 and any(.[0][]; .verificationResult.statement.predicate == $doc[0])'
```

To repeat only the signed subject comparison offline, use the verified bundle and redirect a
previously saved successful `gh release verify "$tag" --repo "$repo" --format json` result:

```bash
bash scripts/ci/verify-release-subjects.sh "$bundle" "$repo" "$tag" "$commit" < release-proof.json
```

Keep that output outside the bundle. The helper compares subjects; it does not authenticate a
JSON file. Do not substitute a downloaded proof or an unverified attestation for the CLI result.

Use `tag=v5.2.1` with the same download procedure to verify the existing root pin. The independent
adapter pin can be verified in a consumer's workspace-free graph only after its immutable release
and public Go module checksums are available. No new root release is required for that pin.
