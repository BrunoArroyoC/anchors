# CLAUDE.md

## Project

**anchors** is a terminal UI for managing Docker containers and Kubernetes pods: list, inspect, and act on them (start/stop/restart, logs, exec, etc.) from one interface. The UI is built with Bubble Tea and **has to look great**: smooth animations, a consistent theme, and polished transitions on every screen are core requirements, not extras.

Status: early. Docker container listing exists in the backend. Kubernetes, the bridge layer, and the screens are still to be built.

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
internal/bridge/         adapter: wraps backend calls as tea.Cmd and defines the tea.Msg types
internal/screen/         UI: Bubble Tea models (Init/Update/View), styles, animations
  app.go                 root model: owns the active screen, routes msgs, switches screens
  theme/                 the palette (paper-white halftone on near-black, international orange accent) + Lip Gloss styles
  dots/                  canvas: Braille dots (2x4 per cell) + per-cell glyphs; ink masks (Art), grayscale tone maps (Tone)
  assets/                embedded PNG art (+ src/ Recraft SVG sources)
  splash/                title screen: halftone anchor on its chain, condensed wordmark, cursor lens/trail -> splash.DoneMsg
```

Dependencies only go one way:

```
screen ──> bridge ──> functions ──> models
   └──────────────────────────────> models
```

### Layer rules

- **functions**: synchronous, takes `context.Context` plus a client, returns `(models.X, error)`. Never import `tea`. Functions the bridge calls must be **exported** (capitalized).
- **bridge**: the only place where backend calls become async. Each operation gets a `tea.Cmd` constructor and its result `Msg` types:
  ```go
  type ContainersLoadedMsg struct{ Containers []models.DockerModel }
  type ErrMsg struct{ Err error }

  func LoadContainers(cli *client.Client) tea.Cmd {
      return func() tea.Msg {
          ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
          defer cancel()
          cs, err := functions.ListContainers(ctx, cli)
          if err != nil {
              return ErrMsg{err}
          }
          return ContainersLoadedMsg{cs}
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
- Container state colors must match across screens (running / exited / paused / restarting / unhealthy).
- Key bindings go through `bubbles/key` with a `help` view so they are discoverable.
- Art style (branch `halftoneBackground`): modern editorial poster. The image is a halftone of round glyphs (`· • ●`) on a 45° screen (every other cell, offset per row), with a fine Braille outline traced from the image in the cells between. Type is a bold condensed grotesk wordmark drawn in Braille; small labels are tracked-out text. `dots.Canvas` emits raw truecolor ANSI (Lip Gloss per cell is too slow at 30fps). Pixel art / half-blocks was tried and rejected by the author. Regular text screens use Lip Gloss + `theme`.
- Motion lives in the halftone and reacts to the cursor; the composition itself stays put. Tick at 30fps while something moves, 12fps when only the background grain is breathing.
- `ANCHORS_REDUCE_MOTION=1` must give a calmer variant of every animated screen (less movement, no flashes), not a blank one.
- Mouse is on (`tea.WithMouseAllMotion`). On the splash the cursor is a lens that swells and magnifies the halftone and leaves an orange trail that cools off; click sends a ripple.

## Art pipeline

1. Generate vector art with the `openrouter-images` skill, model `recraft/recraft-v4-pro-vector` (SVG only, ~$0.30/image). Keep the SVG in `internal/screen/assets/src/`.
2. Rasterize to a PNG ~1000px wide, e.g. with `@resvg/resvg-js` run from a scratch dir (not a project dependency). Halftone images: grayscale, one big bold subject, light on black (fine detail like a distant ship gets lost — a moon+ship scene was unreadable). Lettering: black on white.
3. Embed it in `assets/assets.go`. `dots.LoadTone` turns an image into a tone map for halftones; `dots.Art.Mask` turns lettering/line art into Braille dots.
4. Look at the result: `ANCHORS_SNAPSHOT_DIR=/some/dir go test ./internal/screen/splash/` writes each test frame as a PNG (80x24, 120x36, 200x55). `bench_test.go` measures frame cost.

Ask before adding any new library.

## Conventions

- Go files are camelCase (`dockerFunctions.go`). Keep that for new files.
- Wrap errors with context: `fmt.Errorf("list containers: %w", err)`.
- Don't shadow the `error` builtin (use `err`).
- Create the Docker client once in `main` and pass it down. Don't create it per call.
