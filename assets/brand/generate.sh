#!/usr/bin/env bash
# Regenerate the STH brand icon (Buddy) from the canonical ASCII mark.
#
# The icon is NOT hand-drawn: it is the design-system Buddy box
#   ╭───╮
#   │^.^│
#   ╰───╯
# rendered in JetBrains Mono Bold, monochrome, exactly as `renderBuddyBox`
# does in the Go CLI (cmd/banner.go) and the design system (assets/buddy.js).
# This script renders it to PNG at every size we ship, for the dark (primary)
# and light tiles. Run it whenever the mark or surfaces change.
#
# Small window sizes are clamped by headless Chromium, so we render the master
# tile at 1024 and downscale (LANCZOS) to every other size — this also gives
# cleaner antialiasing on the tiny icons than a direct small render.
#
# Requirements: a Chromium binary, curl, and Python 3 with Pillow. On the STH
# web/CI runner Chromium lives at /opt/pw-browsers/chromium; override with
# $CHROME. Install Pillow with `pip3 install Pillow`.
set -euo pipefail
DIR="$(cd "$(dirname "$0")" && pwd)"
CHROME="${CHROME:-/opt/pw-browsers/chromium}"
FONT_DIR="$DIR/.fonts"
FONT_TTF="$FONT_DIR/JetBrainsMono-Bold.ttf"
FONT_URL="https://raw.githubusercontent.com/JetBrains/JetBrainsMono/v2.304/fonts/ttf/JetBrainsMono-Bold.ttf"

DARK_SIZES=(1024 512 256 128 64 32)
LIGHT_SIZES=(1024 512 256 128)

if [[ ! -f "$FONT_TTF" ]]; then
  mkdir -p "$FONT_DIR"
  echo "Fetching JetBrains Mono Bold (OFL-1.1) …"
  curl -fsSL --retry 3 -o "$FONT_TTF" "$FONT_URL"
fi
FONT_B64="$(base64 -w0 "$FONT_TTF")"

render() {
  local theme="$1" size="$2" out="$3"
  local tileBg tileFg border
  if [[ "$theme" == dark ]]; then
    tileBg="oklch(0.145 0 0)"; tileFg="oklch(0.985 0 0)"; border="oklch(1 0 0 / 0.08)"
  else
    tileBg="oklch(1 0 0)"; tileFg="oklch(0.145 0 0)"; border="oklch(0 0 0 / 0.10)"
  fi
  cat > "$DIR/_render.html" <<HTML
<!doctype html><html><head><meta charset="utf-8"><style>
@font-face{font-family:'JBM';src:url(data:font/ttf;base64,${FONT_B64}) format('truetype');font-weight:700;font-display:block}
*{margin:0;padding:0;box-sizing:border-box}
html,body{width:100%;height:100%;background:${tileBg}}
.tile{position:fixed;inset:0;display:flex;align-items:center;justify-content:center;background:${tileBg}}
.tile::after{content:"";position:absolute;inset:$(awk "BEGIN{print ${size}*0.03}")px;border:$(awk "BEGIN{print ${size}*0.006}")px solid ${border};pointer-events:none}
pre{font-family:'JBM',monospace;font-weight:700;line-height:1;letter-spacing:0;font-feature-settings:normal;white-space:pre;color:${tileFg};font-size:$(awk "BEGIN{print ${size}*0.205}")px}
</style></head><body><div class="tile"><pre id="box"></pre></div>
<script>
function padFace(face,innerWidth){var pad=Math.max(0,innerWidth-face.length);var left=Math.floor(pad/2);return " ".repeat(left)+face+" ".repeat(pad-left);}
function renderBuddyBox(face,innerWidth){var h="─".repeat(innerWidth);return "╭"+h+"╮\n│"+padFace(face,innerWidth)+"│\n╰"+h+"╯";}
document.getElementById('box').textContent=renderBuddyBox('^.^',3);
</script></body></html>
HTML
  "$CHROME" --headless=new --disable-gpu --no-sandbox --hide-scrollbars \
    --force-device-scale-factor=1 --window-size="${size},${size}" \
    --screenshot="$out" "file://$DIR/_render.html" 2>/dev/null
}

render dark  1024 "$DIR/icon-1024.png"
render light 1024 "$DIR/icon-light-1024.png"
rm -f "$DIR/_render.html"

downscale() {
  local src="$1" out="$2" size="$3"
  python3 - "$src" "$out" "$size" <<'PY'
import sys
from PIL import Image
src, out, size = sys.argv[1], sys.argv[2], int(sys.argv[3])
Image.open(src).convert("RGBA").resize((size, size), Image.LANCZOS).save(out)
PY
}

for size in "${DARK_SIZES[@]}";  do [[ "$size" == 1024 ]] || downscale "$DIR/icon-1024.png"       "$DIR/icon-${size}.png"       "$size"; done
for size in "${LIGHT_SIZES[@]}"; do [[ "$size" == 1024 ]] || downscale "$DIR/icon-light-1024.png" "$DIR/icon-light-${size}.png" "$size"; done
echo "Generated:"; ls -1 "$DIR"/icon-*.png