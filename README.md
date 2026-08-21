# Portalis

A built-in terminal emulator for Go. Runs a shell in a PTY, parses ANSI
output, and renders the result through [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Designed to be embedded into a host application that allocates a rectangle
and forwards keyboard, mouse and resize events.

## Features

- PTY-backed session via [`creack/pty`](https://github.com/creack/pty)
- ANSI/VT parser: CSI, OSC, SGR colors (16 / 256 / 24-bit), UTF-8
- xterm-compatible key encoding (Ctrl/Alt/Shift/F-keys, application cursor mode)
- DEC modes: `?1` application cursor, `?25` cursor visibility, `?1049` alt screen, `?2004` bracketed paste, `?2026` synchronized output
- Editing sequences: ICH (`CSI @`), DCH (`CSI P`), ECH (`CSI X`), IL (`CSI L`), DL (`CSI M`), SU (`CSI S`), SD (`CSI T`), VPA (`CSI d`), HPA (`CSI G`)
- Scroll regions, index/reverse index, DEC Special Graphics charset
- OSC 7 working-directory tracking with callbacks
- OSC 52 clipboard integration (macOS, Wayland, X11)
- Synchronized output (`CSI ? 2026 h/l`)
- Selection with mouse drag, scrollback up to 10 000 lines
- Alt screen, bracketed paste, command history
- Render dirty-cache and PTY output coalescing for performance
- Framework-agnostic core: feed events, call `View(w, h)` to render

## Installation

```bash
go get github.com/starframe-dev/portalis
```

Requires Go 1.22 or later.

## Quick start

```go
import (
    tea "github.com/charmbracelet/bubbletea"
    "github.com/starframe-dev/portalis"
)

em := portalis.NewEmulator("session-1", "build", "bash", []string{"-l"})

em.OnCWDChange = func(dir string) {
    fmt.Println("cwd:", dir)
}

p := tea.NewProgram(em, tea.WithAltScreen())
if _, err := p.Run(); err != nil {
    log.Fatal(err)
}
```

The host calls `Update(msg)` with `tea.KeyMsg`, `tea.MouseMsg`, `ResizeMsg`,
`PtyOutputMsg`, `PtyExitMsg`, and `CursorBlinkMsg`. Render any time with
`View(width, height)`.

## Architecture

| File | Responsibility |
|---|---|
| `emulator.go` | Top-level controller. Coordinates Screen, Parser and PTY. |
| `screen.go` | 2D cell grid, scrollback, selection, rendering, dirty cache. |
| `ansi.go` | ANSI/VT escape parser (CSI, OSC, SGR, UTF-8, DEC modes). |
| `pty.go` | PTY spawn / read / write / resize / output coalescing. |
| `clipboard.go` | OSC 52 with platform backends. |

```
┌────────────────── Emulator ──────────────────┐
│  Parser  ──▶  Screen  ◀──  PTY (process I/O) │
└──────────────────────────────────────────────┘
```

## Documentation

Project specs live under `specs/` and per-file specs under `code-specs/`:

- [`specs/portalis.md`](specs/portalis.md) — full architecture overview
- [`code-specs/emulator.md`](code-specs/emulator.md) — public API of `Emulator`
- [`code-specs/screen.md`](code-specs/screen.md) — `Screen` cell grid
- [`code-specs/ansi.md`](code-specs/ansi.md) — parser states
- [`code-specs/pty.md`](code-specs/pty.md) — PTY lifecycle
- [`code-specs/clipboard.md`](code-specs/clipboard.md) — clipboard backends

## Testing

```bash
go test ./...
go test -race ./...
```

Unit tests cover ANSI sequences, screen operations, key encoding, and PTY resize/coalescing. Visual E2E with `tmux` is run via `cuetty-cli` (see `cuetty-artifacts/portalis-tmux/`).

Note: `clipboard_mac_test.go` runs only on macOS; other tests are portable.

## License

MIT (see project root).

---

## About this fork

This is a fork of [starframe-dev/portalis](https://github.com/starframe-dev/portalis).

The only change is the module path. Upstream declares itself as
`github.com/Starframe/portalis`, which no longer resolves — that repository
returns 404, so the module cannot be fetched by its own declared path and every
consumer needs a `replace` directive. A `replace` makes `go install
example.com/consumer@latest` fail outright, so [live-pr](https://github.com/shonenm/live-pr)
depends on this fork instead.

No functional changes. If upstream corrects its module path and tags a release,
this fork becomes unnecessary.
