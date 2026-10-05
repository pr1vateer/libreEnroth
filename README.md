# libre-enroth

A reimplementation of the *Might and Magic VIII: Day of the Destroyer* engine in Go on
[Ebitengine](https://ebitengine.org). It runs on the original game data, which you must supply
from your own copy of the game (for example the GOG release). No game data is included in this
repository.

Status: milestone M6. You get the title screen, credits, character creation and the in-game
screen. The party walks, jumps and flies through every outdoor (`.odm`) and indoor (`.blv`) map,
drawn by a software renderer that reproduces the original's Direct3D look. Time passes, the
party rests, and the map scripts run: doors, clicks, NPC dialogues and houses with their clips.
Taverns, temples, banks, stables and boats work, as do dungeon entrances and walking off the
edge of a map to the next one. Items, skills and stats (shops, training), monsters, combat,
spells, saving and sound are later milestones. "New Game" → OK starts on `out01.odm`.
See `PLAN.md` in the parent repository for the roadmap.

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
UI to fit, keeping its aspect ratio. Widescreen resolutions get black bars at the sides. The 3D
view inside the HUD frame is rendered at the full resolution of the screen area it covers, not
upscaled from 640×480.

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
  - **+/−** beside a stat spend or return points; click two of the offered skills.
- **In game** (free camera; M4 brings real party movement):
  - **W/S** or **↑/↓** move, **A/D** strafe, **←/→** turn, **PgUp/PgDn** look up/down.
  - **Space/C** fly up/down, **Shift** moves 4× faster, right-drag looks around.
  - **F2** (indoors) opens every closed door and closes every open one.
  - **+/−** next to the minimap zoom it (outdoors).
  - **1–5** or a click on a portrait selects a party member; the selected member again
    opens the **character screen**:
    - **S**tats, s**K**ills, **I**nventory, **A**wards pages; **Esc** closes it.
    - Click an item to pick it up, click again to drop it (or to swap); click the paper doll
      to wear the item on the cursor or take one off; the magnifier shows the rings.
    - On the skills page a click on a skill spends skill points on it.
    - Hold the right button on an item for its description (and on a stat or skill).
    - With an item on the cursor, right-click another to mix potions (or a reagent into a
      bottle); drop a potion on the paper doll, or right-click a portrait, to drink it.
  - Chests opened by the map's events show their items: click to take (gold goes to the
    party), click on the chest to put the cursor's item back; the selected portrait again
    shows that member's pack.
  - **Esc** returns to the title (the original's game menu comes later).

## Developer options

| flag | meaning |
|---|---|
| `-state title\|credits\|create\|ingame` | start directly in a screen |
| `-screenshot out.png -frames N` | render N frames, save the last one as PNG, and exit |
| `-mouse x,y` | pin the mouse at a UI position (0..639, 0..479), e.g. to capture hover states |
| `-map name` | start in game on a `games.lod` map, e.g. `out01.odm`, `out02.odm`, `d05.blv`, `eleme.blv` (implies `-state ingame`) |
| `-cam x,y,z,yaw,pitch` | camera position (z of the feet; the eye is 160 higher) and angles in 2048ths of a turn (0 = east, 512 = north) |
| `-time HH:MM` | time of day for the outdoor lighting (default 9:00) |
| `-party 0,5,12` | a party of 1–5 portrait faces (0–27, as in party creation); a created hero replaces the first. Default: one member with face 0 |
| `-equip` | give the party the items of their skills, worn where they fit (debug) |
| `-input script` | replay keys and mouse: `Up:120,1:2,Click@320/200:1,Right@40/60:5,-:30` (`Right@x/y` holds the right button) |

Outdoors the camera starts at the new-game spawn for `out01.odm`, elsewhere above the map
centre. Indoors it starts on the map's "Party Start" marker.

Screenshots also work without a display, through Xvfb:

```sh
xvfb-run -a go run ./cmd/libre-enroth -res 1024x768 -state create -screenshot out/create.png -frames 30
xvfb-run -a go run ./cmd/libre-enroth -res 1280x720 -map d16.blv -screenshot out/d16.png
```

`out/` and `*.png` are gitignored, so game-derived images never get committed.

### Tests

```sh
go vet ./...
go test ./...                        # unit tests only; data-dependent tests are skipped
MM8_DATA=../games_mm8 go test ./...  # plus tests against the real game data
```

The real-data UI and world tests compare SHA-256 hashes of rendered screens. To look at the
images, run:

```sh
MM8_DATA=../games_mm8 go test ./internal/game/ui -run Screens -update      # writes out/ui_*.png
MM8_DATA=../games_mm8 go test ./internal/game/world -run Golden -update    # writes out/world_*.png
MM8_DATA=../games_mm8 go test ./internal/game/world ./internal/render -run X -bench Frame
```

On a 6-core aarch64 machine, a frame takes about 2.5 ms (outdoor out01) and 2.4 ms (indoor d16)
at 1280×720, and about 5 ms at 1920×1080.

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
| `internal/assets/…` | decoders for the original formats (LOD, bitmaps, icons, PCX, fonts, sprites, HWL, txt, the d*.bin descriptor tables) and the exe-table reader |
| `internal/maps/odm`, `internal/maps/blv`, `internal/maps/delta` | outdoor and indoor maps, and their `.ddm`/`.dlv` state files |
| `internal/render` | the software rasteriser: z-buffer, perspective-correct bilinear texturing, vertex and point lights, parallel bands |
| `internal/game/world` | maps turned into render primitives: terrain, buildings, sky, sectors and portals, lights, doors, decorations |
| `internal/gfx`, `internal/gfx/text` | the 640×480 UI compositor and the bitmap-font renderer |
| `internal/display` | render resolution ↔ UI coordinate mapping |
| `internal/game/ui` | widgets and screens (title, credits, character creation, HUD) |
| `internal/engine` | Ebitengine glue: input, scaling, screenshots |

Code ported from game logic carries a `// mm8: 0x…` comment with the address of the original
function in `MM8-Rel.exe`. The reverse-engineering notes those come from live in the parent
repository (`re/notes/`).
