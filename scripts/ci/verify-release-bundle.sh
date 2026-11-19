#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
. "${SCRIPT_DIR}/python-runtime.sh"
. "${SCRIPT_DIR}/release-scope.sh"
repo_python_init

if (( $# != 4 )); then
  echo "usage: $0 <bundle-dir> <owner/repository> <tag> <commit>" >&2
  exit 2
fi

bundle_dir="$1"
release_scope "$2" "$3" "$4"
bash "$SCRIPT_DIR/check-release-module.sh" "$tag" "$commit"

[[ "$(git rev-parse --verify "refs/tags/${tag}^{commit}")" == "$commit" ]] || {
  echo "release bundle: tag target does not match requested commit" >&2
  exit 1
}

"${CI_PYTHON_BIN}" - "$bundle_dir" "$repository" "$tag" "$commit" "$archive" "$sbom" "$module_dir" "$module_path" "$version" "$artifact_name" "$SCRIPT_DIR/schema-1-releases.txt" <<'PY'
import json
import re
import sys
from pathlib import Path

directory, repository, tag, commit, archive, sbom, module_dir, module, version, artifact, legacy_releases = sys.argv[1:]
bundle = Path(directory)
names = {archive, sbom, "release-manifest.json", "SHA256SUMS"}
files = list(bundle.iterdir())
if {p.name for p in files} != names or any(p.is_symlink() or not p.is_file() for p in files):
    raise SystemExit("release bundle: asset set mismatch")
with (bundle / "release-manifest.json").open(encoding="utf-8") as handle:
    manifest = json.load(handle)
if not isinstance(manifest, dict) or type(manifest.get("schemaVersion")) is not int:
    raise SystemExit("release bundle: invalid manifest schema version")
expected = {
    "schemaVersion": 2,
    "repository": repository,
    "ref": f"refs/tags/{tag}",
    "tag": tag,
    "commit": commit,
    "module": module,
    "moduleDirectory": module_dir,
    "version": version,
    "artifactName": artifact,
    "signerWorkflow": f"{repository}/.github/workflows/release.yml",
    "oidcIssuer": "https://token.actions.githubusercontent.com",
    "sourceArchive": archive,
    "sbom": sbom,
}
if manifest["schemaVersion"] == 1:
    if module_dir != "." or f"{tag} {commit}" not in Path(legacy_releases).read_text(encoding="ascii").splitlines():
        raise SystemExit("release bundle: not a published schema-1 tag and commit")
    expected = {k: expected[k] for k in ("repository", "ref", "tag", "commit", "sourceArchive", "sbom")}
    expected["schemaVersion"] = 1
if manifest != expected:
    raise SystemExit("release bundle: manifest identity mismatch")

checksums = (bundle / "SHA256SUMS").read_text(encoding="utf-8").splitlines()
subjects = []
for line in checksums:
    match = re.fullmatch(r"[0-9a-f]{64} [ *](.+)", line)
    if not match:
        raise SystemExit("release bundle: malformed checksum entry")
    subjects.append(match[1])
if sorted(subjects) != sorted(names - {"SHA256SUMS"}):
    raise SystemExit("release bundle: checksum subject set mismatch")

with (bundle / sbom).open(encoding="utf-8") as handle:
    sbom = json.load(handle)
if not str(sbom.get("spdxVersion", "")).startswith("SPDX-2."):
    raise SystemExit("release bundle: SBOM is not SPDX JSON")
if sbom.get("SPDXID") != "SPDXRef-DOCUMENT":
    raise SystemExit("release bundle: SBOM document identity is missing")
PY

(
  cd "$bundle_dir"
  sha256sum --check --strict SHA256SUMS >/dev/null
)

archive_check_dir=$(mktemp -d)
trap 'rm -rf "$archive_check_dir"' EXIT
if ! release_archive >"$archive_check_dir/expected.tar"; then
  echo "release bundle: failed to archive the expected commit" >&2
  exit 1
fi
if ! gzip -cd -- "$bundle_dir/$archive" >"$archive_check_dir/actual.tar"; then
  echo "release bundle: source archive is not valid gzip" >&2
  exit 1
fi
if ! cmp -s "$archive_check_dir/expected.tar" "$archive_check_dir/actual.tar"; then
  echo "release bundle: source archive does not match the manifest commit tree" >&2
  exit 1
fi

if tar -tzf "$bundle_dir/$archive" | awk -v prefix="$artifact_name/" \
  'index($0, prefix) != 1 { exit 1 }'; then
  :
else
  echo "release bundle: source archive contains an unexpected root" >&2
  exit 1
fi

tar -tzf "$bundle_dir/$archive" -- \
  "$artifact_name/${module_prefix}go.mod" "$artifact_name/${module_prefix}go.sum" \
  "$artifact_name/${module_prefix}LICENSE" >/dev/null
