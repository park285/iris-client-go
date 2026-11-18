#!/usr/bin/env bash
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd -P)"
. "${SCRIPT_DIR}/python-runtime.sh"
repo_python_init

if (( $# != 2 )); then
  echo "usage: $0 <tag> <commit>" >&2
  exit 2
fi

"${CI_PYTHON_BIN}" - "$1" "$2" <<'PY'
import json
import re
import subprocess
import sys
import tempfile
from pathlib import Path

tag, commit = sys.argv[1:]
match = re.fullmatch(r"v(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)\.(0|[1-9][0-9]*)", tag)
if match is None:
    raise SystemExit("release module: tag must be strict semver")
if re.fullmatch(r"[0-9a-f]{40}", commit) is None:
    raise SystemExit("release module: commit must be a full lowercase Git SHA")
source = subprocess.check_output(["git", "show", f"{commit}:go.mod"])
with tempfile.TemporaryDirectory(prefix="release-module-") as directory:
    module_file = Path(directory) / "go.mod"
    module_file.write_bytes(source)
    module = json.loads(subprocess.check_output(["go", "mod", "edit", "-json", str(module_file)]))
path = module["Module"]["Path"]
suffix = re.search(r"/v([0-9]+)$", path)
major = int(match[1])
if (suffix is None and major >= 2) or (suffix is not None and (major < 2 or suffix[1] != str(major))):
    raise SystemExit("release module: tag major does not match the committed Go module path")
PY
