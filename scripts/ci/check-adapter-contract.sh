#!/usr/bin/env bash
set -euo pipefail
unset CDPATH

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
[[ -f "$root/adapters/valkeydedup/go.mod" ]] || {
  echo "adapter contract: missing public module adapters/valkeydedup/go.mod" >&2
  exit 1
}
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
export GOWORK=off
sdk=github.com/park285/iris-client-go/v5
adapter=github.com/park285/iris-client-go/adapters/valkeydedup
declared=$(cd "$root/adapters/valkeydedup" && go list -m)
[[ "$declared" == "$adapter" ]] || {
  echo "adapter contract: adapters/valkeydedup declares $declared, want $adapter" >&2
  exit 1
}
cd "$fixture"
go mod init local.test/adapter-consumer
go mod edit -go="$(awk '$1 == "go" { print $2 }' "$root/go.mod")" \
  -require="$sdk@v5.2.1" -require="$adapter@v1.0.0" -replace="$adapter=$root/adapters/valkeydedup"
cat > consumer.go <<'GO'
package consumer

import (
	"github.com/park285/iris-client-go/adapters/valkeydedup"
	"github.com/park285/iris-client-go/v5/webhook"
)

var _ = webhook.WithNonceStore(valkeydedup.NewNonceStore(nil))
GO
go build -mod=mod .
go mod edit -replace="$sdk=$root"
go build -mod=mod .
