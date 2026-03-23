#!/usr/bin/env bash

set -euo pipefail

ROOT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")/.." && pwd)"
OUTPUT_DIR="$ROOT_DIR/docs/screenshots"
FREEZE_CMD=(go run github.com/charmbracelet/freeze@latest)
FREEZE_FLAGS=(
  --window
  --width 1700
  --height 1200
  --background "#07131F"
  --border.color "#27455F"
  --padding 0
  --margin 24
  --font.size 18
  --line-height 1.05
)

mkdir -p "$OUTPUT_DIR"

cd "$ROOT_DIR"

"${FREEZE_CMD[@]}" \
  --execute "go run ./cmd/dbgold-docshot dashboard" \
  --output "$OUTPUT_DIR/dashboard.png" \
  "${FREEZE_FLAGS[@]}"

"${FREEZE_CMD[@]}" \
  --execute "go run ./cmd/dbgold-docshot restore" \
  --output "$OUTPUT_DIR/restore.png" \
  "${FREEZE_FLAGS[@]}"
