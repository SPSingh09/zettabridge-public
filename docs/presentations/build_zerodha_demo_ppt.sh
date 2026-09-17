#!/usr/bin/env bash
set -euo pipefail
ROOT="$(cd "$(dirname "$0")/../.." && pwd)"
cd "$ROOT"
VENV="$ROOT/scripts/.ppt-venv"
if [[ ! -x "$VENV/bin/python" ]]; then
  python3 -m venv "$VENV"
  "$VENV/bin/pip" install -q python-pptx
fi
"$VENV/bin/python" "$ROOT/scripts/build_zerodha_demo_ppt.py"
echo ""
echo "Done. Open:"
echo "  $ROOT/docs/presentations/ZettaBridge_Zerodha_Demo_Final.pptx"
