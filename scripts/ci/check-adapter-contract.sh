#!/usr/bin/env bash
set -euo pipefail

root=$(cd "$(dirname "${BASH_SOURCE[0]}")/../.." && pwd -P)
fixture=$(mktemp -d)
trap 'rm -rf "$fixture"' EXIT
export GOWORK=off
sdk=github.com/park285/iris-client-go/v5
adapter=github.com/park285/iris-client-go/adapters/valkeydedup
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
