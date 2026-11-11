# CLAUDE.md

## Project

**anchors** is a terminal UI for managing Docker containers and Kubernetes pods: list, inspect, and act on them (start/stop/restart, logs, exec, etc.) from one interface. The UI is built with Bubble Tea and **has to look great**: smooth animations, a consistent theme, and polished transitions on every screen are core requirements, not extras.

Status: early. The backend lists Docker containers and images; the bridge wraps both; the splash and the dashboard (containers, images, an empty pods section) are built. Container actions, logs, exec and Kubernetes are still to come.

## Stack

- Go 1.27 (module name: `docker-tool`)
- Docker: `github.com/moby/moby/client` + `github.com/moby/moby/api/types/*` (the new moby module paths, **not** the legacy `github.com/docker/docker/client`)
- TUI: Bubble Tea **v1** (`github.com/charmbracelet/bubbletea`), Bubbles, Lip Gloss, Huh (forms), Glamour (markdown)
- Animations: use `github.com/charmbracelet/harmonica` (spring physics) driven by `tea.Tick` messages. Add it when needed.
- Kubernetes (planned): `k8s.io/client-go`

## Commands

```bash
go run ./cmd/my-cli      # run the app (needs a reachable Docker daemon, DOCKER_HOST is respected)
go build ./...           # build
go vet ./...             # vet
go test ./...            # tests
go mod tidy              # only after the Charm libs are imported, or tidy removes them from go.mod
```

## Architecture

```
cmd/my-cli/main.go       entrypoint: creates clients, builds the root model, runs tea.NewProgram
internal/models/         plain data structs shared by every layer (DockerModel, later PodModel...)
internal/functions/      backend: talks to Docker/K8s APIs, returns models. No Bubble Tea imports.
internal/bridge/         adapter: wraps backend calls as tea.Cmd and defines the tea.Msg types (docker.go)
internal/screen/         UI: Bubble Tea models (Init/Update/View), styles, animations
  app.go                 root model: owns the active screen, routes msgs, switches screens
  theme/                 the palette (paper-white halftone on near-black, international orange accent) + Lip Gloss styles
  dots/                  canvas: Braille dots (2x4 per cell) + per-cell glyphs; ink masks (Art), grayscale tone maps (Tone)
  assets/                embedded PNG art (+ src/ Recraft SVG sources)
  splash/                title screen: halftone anchor on its chain, condensed wordmark, cursor lens/trail -> splash.DoneMsg
  dashboard/             main screen: night scene (stars, perspective sea, lighthouse whose beam follows the selection: scene.go) + the logbook box (summary, tabs, list, detail); key hints under it
```

Dependencies only go one way:

```
screen ──> bridge ──> functions ──> models
   └──────────────────────────────> models
```

### Layer rules

- **functions**: synchronous, takes `context.Context` plus a client, returns `(models.X, error)`. Never import `tea`. Functions the bridge calls must be **exported** (capitalized).
- **bridge**: the only place where backend calls become async. Each operation gets a `tea.Cmd` constructor and one result `Msg` that carries the data or the error, so the screen knows which load failed:
  ```go
  type ContainersMsg struct {
      Containers []models.DockerModel
      Err        error
  }

  func LoadContainers(cli *client.Client) tea.Cmd {
      return func() tea.Msg {
          ctx, cancel := context.WithTimeout(context.Background(), callTimeout) // 5s
          defer cancel()
          cs, err := functions.ListContainers(ctx, cli)
          return ContainersMsg{Containers: cs, Err: err}
      }
  }
  ```
  Streams (logs, stats, events) work like this: a Cmd reads **one** item from a channel and returns it as a Msg, and the screen calls the Cmd again to get the next one.
- **screen**: never calls `functions` directly and never does blocking I/O in `Update`/`View`. It reacts to bridge Msgs. One root model owns the active screen and routes messages and `tea.WindowSizeMsg` to it.

## UI / design guidelines

- Keep all colors, borders, and spacing in a single theme/styles file inside `internal/screen` (Lip Gloss styles). Screens don't hardcode colors.
- Animations are driven by ticks (`tea.Tick` ~60fps while animating, none when idle). Stop ticking once an animation settles so the app doesn't burn CPU.
- Every async load shows a spinner or skeleton state, and every error is shown in the UI. Never `log.Fatal` once the program is running.
- Always handle `tea.WindowSizeMsg` and lay out for small terminals (80x24 minimum).
- Container states look the same on every screen: use `theme.ContainerLook(state, health)` (glyph, color, word). Alive is paper-white `●`, stopped is mist `○`/`◐`, anything that needs a look (unhealthy, restarting, dead) is signal orange.
- Key bindings go through `bubbles/key` in a `keyMap` that satisfies `help.KeyMap`: `ShortHelp` is the footer, `FullHelp` is the `?` sheet. Screens drawn on the canvas render those bindings themselves.
- Art style: the halftone poster is **the project's design**. Every new screen follows it (near-black ground, paper-white halftone, international-orange accent, condensed grotesk type). It is a modern editorial poster. The image is a halftone of round glyphs (`· • ●`) on a 45° screen (every other cell, offset per row), with a fine Braille outline traced from the image in the cells between. Type is a bold condensed grotesk wordmark drawn in Braille; small labels are tracked-out text. `dots.Canvas` emits raw truecolor ANSI (Lip Gloss per cell is too slow at 30fps). Pixel art / half-blocks was tried and rejected by the author. Other directions were explored and not chosen: hand-drawn ink with a particle sea (commit `ffad13a`) and an isometric box grid (branch `boxesBackground`). Regular text screens use Lip Gloss + `theme`.
- Motion lives in the halftone and reacts to the cursor; the composition itself stays put. Tick at 30fps while something moves, 12fps when only the background grain is breathing.
- Dashboard (branch `lighthouseDesing`, "night watch"; earlier variants: `shipBehindBox` ship watermark, `freeDashBoardVersion` ship column with live cargo, `dashBoardMotionBackground` plain box over the sea, `dashboardList` full-width list): the manifest is a logbook panel (`theme.Panel`, `theme.Frame` hairline, name in the top edge, sync state in the bottom edge) on the left of a night scene (`scene.go`): stars in the sky, a halftone sea in perspective below a horizon at 70% of the height (dots grow toward the viewer, swells roll in while you work), and a stippled lighthouse (`assets/lighthouse.png`, measured constants in `scene.go`) standing on the horizon to the right.
- The lighthouse beam is the signature motion: it swings (about 0.3s) to point at the selected row on the logbook's right edge, which catches the light; it is warm white, or signal orange when the selected container needs a look; the lantern and its reflection on the sea take the same colour. While you work it drifts slightly; `r` makes it sweep the sea and sends a ripple from the rocks. The lighthouse appears from 100x24 up; smaller terminals get the logbook under the stars.
- Dashboard motion (all of it settles ~4.5s after the last key or mouse event, then the frame loop stops): the box unfolds from its middle on open, rows print in from halftone dots, the beam swings to the selection, the selection bar (Braille, quarter-row steps) and the tab underline glide, running containers breathe, a container that changes state glows orange, the sea swells, parts under the cursor and keeps an orange wake, clicking the sea or pressing `r` sends a ripple. Data refreshes every 5s without waking the frame loop unless something changed.
- The dashboard loads containers and images while the splash is up (`App` routes bridge msgs to it), so it opens with data.
- `ANCHORS_REDUCE_MOTION=1` must give a calmer variant of every animated screen (less movement, no flashes), not a blank one.
- Mouse is on (`tea.WithMouseAllMotion`). On the splash the cursor is a lens that swells and magnifies the halftone and leaves an orange trail that cools off; click sends a ripple.

## Art pipeline

1. Generate vector art with the `openrouter-images` skill, model `recraft/recraft-v4-pro-vector` (SVG only, ~$0.30/image). Keep the SVG in `internal/screen/assets/src/`.
2. Rasterize to a PNG ~1000px wide, e.g. with `@resvg/resvg-js` run from a scratch dir (not a project dependency). Halftone images: grayscale, one big bold subject, light on black (fine detail like a distant ship gets lost — a moon+ship scene was unreadable). Lettering: black on white.
3. Embed it in `assets/assets.go`. `dots.LoadTone` turns an image into a tone map for halftones; `dots.Art.Mask` turns lettering/line art into Braille dots.
4. Look at the result: `ANCHORS_SNAPSHOT_DIR=/some/dir go test ./internal/screen/splash/` writes each test frame as a PNG (80x24, 120x36, 200x55). `bench_test.go` measures frame cost. The dashboard test writes raw ANSI frames (`.ans`) instead; render them with any ANSI-to-image tool (or `cat` one in a truecolor terminal).

Ask before adding any new library.

## Conventions

- Go files are camelCase (`dockerFunctions.go`). Keep that for new files.
- Wrap errors with context: `fmt.Errorf("list containers: %w", err)`.
- Don't shadow the `error` builtin (use `err`).
- Create the Docker client once in `main` and pass it down. Don't create it per call.
