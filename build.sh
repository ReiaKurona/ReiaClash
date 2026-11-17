#!/bin/sh
set -eu
cd "$(dirname "$0")"
go build -o reia-service ./cmd/reia-service
go build -o reia-agent ./cmd/reia-agent
go build -o reia-tray ./cmd/reia-tray
go build -o reia ./cmd/reia
if [ "$(uname -s)" = Darwin ]; then
  go run ./scripts/package-macos -dir .
fi
