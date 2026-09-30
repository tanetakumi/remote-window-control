#!/bin/bash
set -euo pipefail
SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]:-$0}")/.." && pwd)"
cd "$SCRIPT_DIR/control-server"
RWC_GO_PATH="$(command -v "${RWC_GO_EXE:-go}")"
RWC_GO_VERSION="$("$RWC_GO_PATH" version)"
RWC_REQUIRED_GO="$(awk '/^go / {print $2}' "$SCRIPT_DIR/control-server/go.mod")"
[[ "$RWC_GO_VERSION" == *"go$RWC_REQUIRED_GO "* ]] || { echo "Expected Go $RWC_REQUIRED_GO; resolved $RWC_GO_PATH ($RWC_GO_VERSION)" >&2; exit 1; }
"$RWC_GO_PATH" run ./cmd/share-host
