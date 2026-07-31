#!/bin/sh
# Generates runtime frontend config and injects base href

CONFIG_FILE="${CONFIG_FILE:-/app/web/dist/config.js}"
INDEX_FILE="${INDEX_FILE:-/app/web/dist/index.html}"
BASE_PATH="${DCS_UI_PATH:-/ui/}"
API_BASE_URL="${DCS_API_PATH:-/}"

if [ "${BASE_PATH#"/"}" = "$BASE_PATH" ]; then
  BASE_PATH="/${BASE_PATH}"
fi

case "$BASE_PATH" in
  */) ;;
  *) BASE_PATH="${BASE_PATH}/" ;;
esac

cat > "$CONFIG_FILE" << EOF
window.DCS_CONFIG = {
  API_BASE_URL: '${API_BASE_URL}',
}
EOF

if [ -f "$INDEX_FILE" ]; then
  sed -i "s|__DCS_UI_BASE_PATH__|${BASE_PATH}|g" "$INDEX_FILE"
fi
