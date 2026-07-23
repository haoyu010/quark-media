#!/bin/sh
set -e
CFG="${QM_CONFIG:-/app/config/config.yaml}"
export QM_STRM_ROOT="${QM_STRM_ROOT:-/app/strm}"
DATA_DIR="/app/data"
CFG_DIR="$(dirname "$CFG")"

# Always ensure persistent dirs exist (bind mounts must already point here)
mkdir -p "$CFG_DIR" "$DATA_DIR" "$QM_STRM_ROOT" "$DATA_DIR/mtproto" "$DATA_DIR/backups"

# Seed ONLY when missing — never overwrite user data
if [ ! -f "$CFG" ]; then
  echo "[entrypoint] seed config -> $CFG"
  cp /app/config.example.yaml "$CFG"
fi
if [ ! -f "$DATA_DIR/category.yaml" ] && [ -f /app/category.yaml ]; then
  echo "[entrypoint] seed category.yaml"
  cp /app/category.yaml "$DATA_DIR/category.yaml"
fi
if [ ! -f "$DATA_DIR/quark_config.json" ]; then
  echo "[entrypoint] seed quark_config.json"
  echo '{"cookie":[],"tasklist":[],"task_settings":{},"push_config":{},"telegram_source":{}}' > "$DATA_DIR/quark_config.json"
fi

# Writable for container + host tools
chmod -R a+rwX "$CFG_DIR" "$DATA_DIR" "$QM_STRM_ROOT" 2>/dev/null || true

echo "[entrypoint] persist cfg=$CFG data=$DATA_DIR strm=$QM_STRM_ROOT"
echo "[entrypoint] cfg_size=$(wc -c < "$CFG" 2>/dev/null || echo 0) qas_size=$(wc -c < "$DATA_DIR/quark_config.json" 2>/dev/null || echo 0)"

exec /app/quark-media -c "$CFG" "${@:-serve}"
