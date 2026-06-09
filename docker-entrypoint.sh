#!/bin/sh
set -eu

RUNTIME_BIN="/app/runtime/cloud-vm-manager"

if [ -x "$RUNTIME_BIN" ]; then
  exec "$RUNTIME_BIN"
fi

exec /app/cloud-vm-manager
