# libre-enroth

A reimplementation of the *Might and Magic VIII: Day of the Destroyer* engine in Go on
[Ebitengine](https://ebitengine.org). It runs on the original game data, which you must supply
from your own copy of the game (for example the GOG release). No game data is included in this
repository.

Status: milestone M2. You get the title screen, credits, character creation and the in-game HUD
frame. There is no 3D world yet (M3), so "New Game" → OK leads to an empty viewport. See
`PLAN.md` in the parent repository for the roadmap.

## Requirements

- **Go 1.27** or newer.
- **A C toolchain and graphics headers**, which Ebitengine uses through cgo.
  - Linux: gcc plus the X11/OpenGL development packages. On Fedora that is
    `libX11-devel libXrandr-devel libXcursor-devel libXinerama-devel libXi-devel libXxf86vm-devel mesa-libGL-devel`.
    On Debian/Ubuntu it is `libx11-dev libxrandr-dev libxcursor-dev libxinerama-dev libxi-dev libxxf86vm-dev libgl1-mesa-dev`.
  - Windows and macOS need nothing extra beyond Go (and Xcode command-line tools on macOS).
- **The MM8 install directory**, the one that contains `MM8-Rel.exe` and `Data/` (`icons.lod`,
  `bitmaps.lod`, `EnglishT.lod`, `EnglishD.lod`, …). libre-enroth only reads these files and
  never modifies them.

## Running

From this directory:

```sh
go run ./cmd/libre-enroth -data /path/to/mm8
```

If you leave out `-data`, the game directory is `$MM8_DATA`. With neither set, it exits
with an error. To set it once for the shell:

```sh
export MM8_DATA=/path/to/mm8
go run ./cmd/libre-enroth -res 1024x768
```

To build a binary instead:

```sh
go build -o libre-enroth ./cmd/libre-enroth
./libre-enroth -data /path/to/mm8 -res 1920x1080 -fullscreen
```

### Resolution and display

The original UI is 640×480. libre-enroth renders at any resolution you choose and scales the
UI to fit, keeping its aspect ratio. Widescreen resolutions get black bars at the sides. (From
M3 on, the 3D view will render at the full resolution.)

| flag | default | meaning |
|---|---|---|
| `-res WxH` or `-res auto` | `640x480` | render resolution; `auto` follows the window size (HiDPI aware) |
| `-window WxH` | same as `-res` | initial window size (the window can be resized) |
| `-fullscreen` | off | start in fullscreen |
| `-filter sharp\|nearest\|linear` | `sharp` | how the UI is scaled; `sharp` keeps pixels crisp at non-integer scales |

Examples:

```sh
go run ./cmd/libre-enroth -res 1280x960                 # exact 2x, pixel-perfect
go run ./cmd/libre-enroth -res 1920x1080 -fullscreen
go run ./cmd/libre-enroth -res auto -window 1280x800     # resize the window freely
```

### Controls

- **Mouse:** everything. A button fires when you press and release on it.
- **Title screen:** **N** New Game, **L** Load (not implemented yet), **C** Credits, **Q** Quit.
- **Credits:** any click or **Esc** returns to the title.
- **Character creation:**
  - Type to enter the name; **Backspace** deletes.
  - The arrows next to "Portrait" change the face and, with it, the class.
  - **Enter** = OK, **Esc** = Cancel, **C** = Clear.
  - Stat points and skill choices are not implemented yet (M7).
- **In game:** **Esc** returns to the title (the original's game menu comes later).

## Developer options

| flag | meaning |
|---|---|
| `-state title\|credits\|create\|ingame` | start directly in a screen |
| `-screenshot out.png -frames N` | render N frames, save the last one as PNG, and exit |
| `-mouse x,y` | pin the mouse at a UI position (0..639, 0..479), e.g. to capture hover states |

Screenshots also work without a display, through Xvfb:

```sh
xvfb-run -a go run ./cmd/libre-enroth -res 1024x768 -state create -screenshot out/create.png -frames 30
```

`out/` and `*.png` are gitignored, so game-derived images never get committed.

### Tests

```sh
go vet ./...
go test ./...                        # unit tests only; data-dependent tests are skipped
MM8_DATA=../games_mm8 go test ./...  # plus tests against the real game data
```

The real-data UI tests compare SHA-256 hashes of rendered screens. To look at the images, run:

```sh
MM8_DATA=../games_mm8 go test ./internal/game/ui -run Screens -update   # writes out/ui_*.png
```

### Asset tool

`lodtool` lists and extracts the game archives:

```sh
go run ./cmd/lodtool ls  ../games_mm8/Data/icons.lod
go run ./cmd/lodtool png ../games_mm8/Data/EnglishD.lod 'T_*' -o out/icons
go run ./cmd/lodtool x   ../games_mm8/Data/EnglishT.lod global.txt -o out
```

## Layout

| path | contents |
|---|---|
| `cmd/libre-enroth` | the game |
| `cmd/lodtool` | archive tool |
| `internal/assets/…` | decoders for the original formats (LOD, bitmaps, icons, PCX, fonts, sprites, HWL, txt) and the exe-table reader |
| `internal/gfx`, `internal/gfx/text` | the 640×480 UI compositor and the bitmap-font renderer |
| `internal/display` | render resolution ↔ UI coordinate mapping |
| `internal/game/ui` | widgets and screens (title, credits, character creation, HUD) |
| `internal/engine` | Ebitengine glue: input, scaling, screenshots |

Code ported from game logic carries a `// mm8: 0x…` comment with the address of the original
function in `MM8-Rel.exe`. The reverse-engineering notes those come from live in the parent
repository (`re/notes/`).
