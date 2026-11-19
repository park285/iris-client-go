#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
. "${SCRIPT_DIR}/python-runtime.sh"
. "${SCRIPT_DIR}/release-scope.sh"
repo_python_init

if (( $# != 2 )); then
  echo "usage: $0 <tag> <commit>" >&2
  exit 2
fi

release_scope park285/iris-client-go "$1" "$2"
"${CI_PYTHON_BIN}" - "$commit" "$module_prefix" "$module_path" <<'PY'
import json
import subprocess
import sys
import tempfile
from pathlib import Path

commit, prefix, expected = sys.argv[1:]
for name in ("go.mod", "go.sum", "LICENSE"):
    path = prefix + name
    entry = subprocess.check_output(["git", "ls-tree", commit, "--", path])
    if not entry.startswith(b"100644 blob "):
        raise SystemExit(f"release module: missing or invalid committed {path}")
source = subprocess.check_output(["git", "show", f"{commit}:{prefix}go.mod"])
with tempfile.TemporaryDirectory(prefix="release-module-") as directory:
    module_file = Path(directory) / "go.mod"
    module_file.write_bytes(source)
    module = json.loads(subprocess.check_output(["go", "mod", "edit", "-json", str(module_file)]))
if module["Module"]["Path"] != expected:
    raise SystemExit(f"release module: expected committed module path {expected}")
if module.get("Replace"):
    raise SystemExit("release module: committed go.mod contains replace directives")
PY
