# Portalis

A built-in terminal emulator for Go. Runs a shell in a PTY, parses ANSI
output, and renders the result through [Bubble Tea](https://github.com/charmbracelet/bubbletea).
Designed to be embedded into a host application that allocates a rectangle
and forwards keyboard, mouse and resize events.

## Features

- PTY-backed session via [`creack/pty`](https://github.com/creack/pty)
- ANSI parser: CSI, OSC, SGR colors (16 / 256 / 24-bit), UTF-8
- OSC 7 working-directory tracking with callbacks
- OSC 52 clipboard integration (macOS, Wayland, X11)
- Synchronized output (`CSI ? 2026 h/l`)
- Selection with mouse drag, scrollback up to 10 000 lines
- Alt screen, bracketed paste, command history
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
| `screen.go` | 2D cell grid, scrollback, selection, rendering. |
| `ansi.go` | ANSI escape parser (CSI, OSC, SGR, UTF-8). |
| `pty.go` | PTY spawn / read / write / resize. |
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

Rendered HTML documentation: [`docs/en/`](docs/en/).

## Testing

```bash
go test ./...
```

Note: `clipboard_mac_test.go` runs only on macOS; other tests are portable.

## License

MIT (see project root).