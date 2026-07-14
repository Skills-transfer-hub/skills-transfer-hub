# STH brand icon — Buddy

<p align="center">
  <img src="icon-256.png" alt="STH Buddy icon" width="128" height="128">
</p>

The STH icon is **Buddy**, the brand's ASCII terminal mascot — not a hand-drawn
graphic. It is the canonical box

```
╭───╮
│^.^│
╰───╯
```

rendered in **JetBrains Mono Bold**, monochrome, on a neutral tile. This mirrors
`Buddy.renderBuddyBox("^.^", 3)` from the design system (`assets/buddy.js`) and
the Go CLI launch banner (`cmd/banner.go`, logo framing, inner width 3). The face
`^.^` is the design-system logo face.

Per the design system: the mark is **never recolored, distorted, or hand-redrawn**
— always regenerate it from the canonical data with `generate.sh`.

## Files

- `icon-{1024,512,256,128,64,32}.png` — dark tile (primary), near-white mark on
  near-black. Use these everywhere by default; they read cleanly down to 32px.
- `icon-light-{1024,512,256,128}.png` — light tile variant for light surfaces.
- `generate.sh` — regenerates every PNG from the canonical mark.

Chocolatey embeds `icon-256.png`; the README header uses `icon-256.png`.

## Regenerate

```sh
pip3 install Pillow          # one-time
./generate.sh                # needs Chromium + curl + Python 3/Pillow
```

`generate.sh` fetches JetBrains Mono Bold (OFL-1.1, pinned) into `.fonts/`
(git-ignored), renders the master tiles at 1024 with headless Chromium, then
downscales the smaller sizes with LANCZOS.
