#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/.." && pwd)"

RWC_GO_PATH="$(command -v "${RWC_GO_EXE:-go}")"
RWC_GO_VERSION="$("$RWC_GO_PATH" version)"
RWC_REQUIRED_GO="$(awk '/^go / {print $2}' "$SCRIPT_DIR/control-server/go.mod")"
[[ "$RWC_GO_VERSION" == *"go$RWC_REQUIRED_GO "* ]] || { echo "Expected Go $RWC_REQUIRED_GO; resolved $RWC_GO_PATH ($RWC_GO_VERSION)" >&2; exit 1; }

RWC_REQUIRED_NODE="$(cat "$SCRIPT_DIR/.node-version")"
[[ "$(node --version)" == "v$RWC_REQUIRED_NODE" ]] || { echo "Expected Node $RWC_REQUIRED_NODE" >&2; exit 1; }

echo "1/3 Building web UI..."
cd "$SCRIPT_DIR/web-ui"
npm install
npm run build

echo "2/3 Building window capture..."
cd "$SCRIPT_DIR"
dotnet build "window-capture/apps/CaptureProbe/CaptureProbe.csproj" -p:EnableWindowsTargeting=true

echo "3/3 Building control server..."
cd "$SCRIPT_DIR/control-server"
"$RWC_GO_PATH" version
GOOS=windows GOARCH=amd64 "$RWC_GO_PATH" build -o share-host.exe ./cmd/share-host

echo "Done."
