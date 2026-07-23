#!/bin/sh
set -e
CFG="${QM_CONFIG:-/app/config/config.yaml}"
export QM_STRM_ROOT="${QM_STRM_ROOT:-/app/strm}"
if [ ! -f "$CFG" ]; then
  echo "[entrypoint] seed config -> $CFG"
  mkdir -p "$(dirname "$CFG")"
  cp /app/config.example.yaml "$CFG"
fi
mkdir -p /app/data "$QM_STRM_ROOT" /app/data/mtproto
if [ ! -f /app/data/category.yaml ] && [ -f /app/category.yaml ]; then
  cp /app/category.yaml /app/data/category.yaml
fi
exec /app/quark-media -c "$CFG" "${@:-serve}"
