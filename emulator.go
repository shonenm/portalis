package portalis

import (
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
)

// asciiArtIcon is shown when the chat session has been stopped.
const asciiArtIcon = `
     _____
    /     \
   /  AI   \
  /_________\
   |  | |  |
   |  | |  |
   \______/
`

// ResizeMsg is sent by a host when the emulator's allocated rectangle changes.
// It carries the content size in cells (without borders or padding).
type ResizeMsg struct {
	Width  int
	Height int
}

// Emulator is a built-in terminal emulator. It runs a shell/command in a PTY
// and renders the output. It is independent of any UI framework; hosts feed it
// keyboard/mouse/resize messages and call View(width, height) to render it.
type Emulator struct {
	SessionID string
	ChatName  string
	cmd       string
	args      []string

	screen  *Screen
	parser  *Parser
	pty     *Pty
	focused bool

	width  int
	height int

	// stopped indicates the underlying session has been terminated and the
	// panel should show the idle ASCII art icon instead of terminal output.
	stopped bool

	// cwd is the last reported working directory (via OSC 7).
	cwd string

	// commandHistory holds commands entered in this terminal.
	commandHistory []string

	// Callbacks invoked when cwd or command history changes.
	OnCWDChange             func(string)
	OnCommandHistoryChanged func([]string)

	// initialCWD is set before Start and used to chdir the PTY process.
	initialCWD string

	// scrollbackLimit caps the screen scrollback buffer.
	scrollbackLimit int

	// Drag-select state: remember the press position and whether an
	// actual drag is in progress. Selection starts only when the mouse
	// moves more than one cell from the press position.
	pressX, pressY int
	dragSelecting  bool

	mu sync.RWMutex
}

// NewEmulator creates a new terminal emulator for the given session.
// If command is empty, it tries to find a shell (bash, sh).

func NewEmulator(sessionID, chatName, command string, args []string) *Emulator {
	if command == "" {
		command, args = defaultShell()
	}
	return &Emulator{
		SessionID: sessionID,
		ChatName:  chatName,
		cmd:       command,
		args:      args,
	}
}

// Start begins spawning the PTY process. Returns PtyReadyMsg when done.
func (e *Emulator) Start() tea.Cmd {
	return e.StartWithEnv(nil)
}

// SetScrollbackLimit sets the maximum number of scrollback lines. Non-positive
// values disable the limit. Call before Start to take effect; calling after
// Start updates the existing screen immediately.
func (e *Emulator) SetScrollbackLimit(limit int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.scrollbackLimit = limit
	if e.screen != nil {
		e.screen.SetScrollbackLimit(limit)
	}
}

// StartSync spawns the PTY process synchronously with extra environment variables.
// Unlike StartWithEnv, it does not return a tea.Cmd — the PTY is ready immediately.
// Returns an error if spawning fails.
func (e *Emulator) StartSync(extraEnv []string) error {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.pty != nil {
		return nil
	}

	// Reset stopped state so the view renders the terminal again.
	e.stopped = false

	e.screen = NewScreen(24, 80)
	if e.scrollbackLimit != 0 {
		e.screen.SetScrollbackLimit(e.scrollbackLimit)
	}
	e.parser = NewParser(e.screen)
	e.parser.SetCWDCallback(func(path string) {
		e.mu.Lock()
		if path != e.cwd {
			e.cwd = path
			if e.OnCWDChange != nil {
				e.OnCWDChange(path)
			}
		}
		e.mu.Unlock()
	})

	pty, err := e.spawnPty(extraEnv)
	if err != nil {
		return err
	}
	e.pty = pty
	if e.width > 0 && e.height > 0 {
		e.screen.Resize(e.height, e.width)
		pty.Resize(e.height, e.width)
	}
	return nil
}

// StartWithEnv begins spawning the PTY process with extra environment variables.
func (e *Emulator) StartWithEnv(extraEnv []string) tea.Cmd {
	return func() tea.Msg {
		e.mu.Lock()
		defer e.mu.Unlock()

		if e.pty != nil {
			return PtyReadyMsg{SessionID: e.SessionID, AlreadyRunning: true}
		}

		// Reset stopped state so the view renders the terminal again.
		e.stopped = false

		e.screen = NewScreen(24, 80)
		if e.scrollbackLimit != 0 {
			e.screen.SetScrollbackLimit(e.scrollbackLimit)
		}
		e.parser = NewParser(e.screen)
		e.parser.SetCWDCallback(func(path string) {
			e.mu.Lock()
			if path != e.cwd {
				e.cwd = path
				if e.OnCWDChange != nil {
					e.OnCWDChange(path)
				}
			}
			e.mu.Unlock()
		})

		pty, err := e.spawnPty(extraEnv)
		if err != nil {
			return PtyExitMsg{SessionID: e.SessionID, Err: err}
		}
		e.pty = pty
		if e.width > 0 && e.height > 0 {
			e.screen.Resize(e.height, e.width)
			pty.Resize(e.height, e.width)
		}
		return PtyReadyMsg{SessionID: e.SessionID}
	}
}

func (e *Emulator) spawnPty(extraEnv []string) (*Pty, error) {
	env := append([]string{"AUTOMATA_SESSION_ID=" + e.SessionID}, extraEnv...)

	// Configure the shell to emit OSC 7 with the current directory after each
	// prompt. Bash reads PROMPT_COMMAND from the environment. echo -e is used
	// instead of printf because printf without a trailing newline can leave the
	// cursor on the same line and interact poorly with the prompt redraw.
	osc7Cmd := `echo -ne "\033]7;${PWD}\007"`
	if e.cmd == "/bin/bash" || e.cmd == "/usr/bin/bash" || strings.HasSuffix(e.cmd, "/bash") {
		env = append(env, "PROMPT_COMMAND="+osc7Cmd)
	}

	if e.initialCWD != "" {
		return SpawnInDir(e.cmd, e.args, e.initialCWD, env...)
	}
	return Spawn(e.cmd, e.args, env...)
}

// Listen returns a command that waits for PTY output.
func (e *Emulator) Listen() tea.Cmd {
	if e.pty == nil {
		return nil
	}
	return listenPty(e.SessionID, e.pty)
}

// DefaultShell returns a usable login shell, preferring absolute paths so it
// works even when the parent PTY provides a minimal or empty PATH.
func DefaultShell() (string, []string) {
	for _, shell := range []string{"/bin/bash", "/usr/bin/bash", "/bin/zsh", "/usr/bin/zsh", "/bin/sh", "/usr/bin/sh"} {
		if _, err := os.Stat(shell); err == nil {
			return shell, []string{"-l"}
		}
	}
	if _, err := exec.LookPath("bash"); err == nil {
		return "bash", []string{"-l"}
	}
	if _, err := exec.LookPath("sh"); err == nil {
		return "sh", []string{"-l"}
	}
	return "sh", nil
}

func defaultShell() (string, []string) {
	return DefaultShell()
}

// Pty returns the underlying PTY for debug purposes.
func (e *Emulator) Pty() *Pty {
	e.mu.RLock()
	defer e.mu.RUnlock()
	return e.pty
}

// View renders the terminal screen at the given panel size. The screen size is
// kept in sync by ResizeMsg from the warp layout engine; during a drag the
// panel is filled by warp's padContent and the real resize happens on release.
func (e *Emulator) View(width, height int) string {
	e.mu.Lock()
	defer e.mu.Unlock()

	if e.stopped {
		return renderAsciiArt(width, height)
	}
	if e.screen == nil {
		return emptyView(width, height)
	}

	return e.screen.Render()
}

// Update handles messages.
func (e *Emulator) Update(msg tea.Msg) tea.Cmd {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		return e.handleKey(msg)
	case tea.MouseMsg:
		return e.handleMouse(msg)
	case tea.WindowSizeMsg:
		return e.handleResize(msg)
	case ResizeMsg:
		return e.handlePanelResize(msg)
	case PtyReadyMsg:
		if msg.SessionID != e.SessionID {
			return nil
		}
		return e.Listen()
	case CursorBlinkMsg:
		e.mu.Lock()
		if e.screen != nil {
			e.screen.CursorBlinkVisible = !e.screen.CursorBlinkVisible
		}
		e.mu.Unlock()
		return nil
	case PtyOutputMsg:
		if msg.SessionID != e.SessionID {
			return nil
		}
		// Feed the parser without holding the mutex: the parser callback may
		// call back into emulator methods that also take the mutex.
		e.mu.Lock()
		parser := e.parser
		e.mu.Unlock()
		if parser != nil {
			parser.Feed(msg.Data)
		}
		// Do not reset the scrollback view here: the user may be reading
		// history while new output arrives. The view is reset on the next
		// keystroke instead (see handleKey).
		// Continue listening
		if e.pty != nil {
			return e.Listen()
		}
		return nil
	case PtyExitMsg:
		if msg.SessionID != e.SessionID {
			return nil
		}
		return nil
	}
	return nil
}

func (e *Emulator) handleKey(msg tea.KeyMsg) tea.Cmd {
	// Ctrl+V (Cmd+V on macOS via most terminals) → paste clipboard.
	if msg.Type == tea.KeyCtrlV {
		return e.handlePaste()
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	// Any keystroke returns the view to the live screen.
	if e.screen != nil {
		e.screen.ResetView()
	}

	if e.pty == nil {
		return nil
	}
	data := keyToBytes(msg)
	if len(data) == 0 {
		return nil
	}

	// Capture the command line before sending Enter to the PTY.
	if msg.Type == tea.KeyEnter && e.screen != nil {
		line := e.screen.LineText(e.screen.Cursor.Row)
		cmd := stripPrompt(line)
		if cmd != "" && (len(e.commandHistory) == 0 || e.commandHistory[len(e.commandHistory)-1] != cmd) {
			const maxHistory = 1000
			e.commandHistory = append(e.commandHistory, cmd)
			if len(e.commandHistory) > maxHistory {
				e.commandHistory = e.commandHistory[len(e.commandHistory)-maxHistory:]
			}
			if e.OnCommandHistoryChanged != nil {
				e.OnCommandHistoryChanged(e.commandHistory)
			}
		}
	}

	// Write synchronously to preserve keystroke order. Async tea.Cmd
	// execution can reorder rapid successive key messages.
	e.pty.Write(data)
	return nil
}

// stripPrompt removes the shell prompt prefix from a terminal line.
// It looks for the last occurrence of common prompt ending characters.
func stripPrompt(line string) string {
	// Find the last prompt marker: $, #, >, or %.
	best := -1
	for _, ch := range []string{"$ ", "# ", "> ", "% "} {
		if idx := strings.LastIndex(line, ch); idx >= 0 && idx+len(ch) > best {
			best = idx + len(ch)
		}
	}
	if best < 0 {
		return strings.TrimSpace(line)
	}
	return strings.TrimSpace(line[best:])
}

func (e *Emulator) scrollUp(lines int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.screen != nil {
		e.screen.ScrollViewUp(lines)
	}
}

func (e *Emulator) scrollDown(lines int) {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.screen != nil {
		e.screen.ScrollViewDown(lines)
	}
}

func (e *Emulator) resetView() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.screen != nil {
		e.screen.ResetView()
	}
}

func (e *Emulator) handleMouse(msg tea.MouseMsg) tea.Cmd {
	switch msg.Button {
	case tea.MouseButtonWheelUp:
		e.scrollUp(3)
		return nil
	case tea.MouseButtonWheelDown:
		e.scrollDown(3)
		return nil
	}

	e.mu.Lock()
	defer e.mu.Unlock()

	switch msg.Action {
	case tea.MouseActionPress:
		if msg.Button == tea.MouseButtonLeft && e.screen != nil {
			// Remember the press position but don't start selection
			// yet — that requires actual drag motion. This keeps
			// simple clicks (used by ai-knowledge) working.
			e.pressX, e.pressY = msg.X, msg.Y
			e.dragSelecting = false
		}
	case tea.MouseActionMotion:
		if msg.Button == tea.MouseButtonLeft && e.screen != nil {
			if !e.dragSelecting {
				dx := msg.X - e.pressX
				if dx < 0 {
					dx = -dx
				}
				dy := msg.Y - e.pressY
				if dy < 0 {
					dy = -dy
				}
				if dx > 1 || dy > 1 {
					e.screen.StartSelection(e.pressY, e.pressX)
					e.dragSelecting = true
				}
			}
			if e.dragSelecting {
				e.screen.ExtendSelection(msg.Y, msg.X)
				return nil
			}
		}
		// Drop other motion to avoid PTY noise.
		return nil
	case tea.MouseActionRelease:
		if msg.Button == tea.MouseButtonLeft {
			if e.screen != nil && e.dragSelecting {
				lines := e.screen.SelectionText()
				e.screen.ClearSelection()
				if len(lines) > 0 {
					copyToClipboard(lines)
				}
			}
			e.dragSelecting = false
		}
	}

	// Forward all events (including Press for left button) to PTY so
	// apps like ai-knowledge receive complete click sequences.
	if e.pty == nil {
		return nil
	}
	data := mouseToBytes(msg)
	if len(data) > 0 {
		e.pty.Write(data)
	}
	return nil
}

// mouseToBytes encodes a bubbletea mouse event as an SGR mouse sequence so
// that TUI applications running inside the PTY (e.g. ai-knowledge) receive
// distinct press, release and wheel events.
func mouseToBytes(msg tea.MouseMsg) []byte {
	var cb int
	switch msg.Button {
	case tea.MouseButtonLeft:
		cb = 0
	case tea.MouseButtonMiddle:
		cb = 1
	case tea.MouseButtonRight:
		cb = 2
	case tea.MouseButtonWheelUp:
		cb = 4
	case tea.MouseButtonWheelDown:
		cb = 5
	default:
		// Release with no button information.
		cb = 3
	}

	if msg.Action == tea.MouseActionMotion {
		cb |= 0b0010_0000
	}
	if msg.Shift {
		cb |= 0b0000_0100
	}
	if msg.Alt {
		cb |= 0b0000_1000
	}
	if msg.Ctrl {
		cb |= 0b0001_0000
	}

	// SGR: ESC [ < Cb ; Cx ; Cy (M for press, m for release)
	suffix := 'M'
	if msg.Action == tea.MouseActionRelease {
		suffix = 'm'
	}
	x := msg.X + 1
	y := msg.Y + 1
	seq := fmt.Sprintf("\x1b[<%d;%d;%d%c", cb, x, y, suffix)
	return []byte(seq)
}

func (e *Emulator) handleResize(msg tea.WindowSizeMsg) tea.Cmd {
	// WindowSizeMsg carries the full window size; panel size comes via ResizeMsg.
	return nil
}

func (e *Emulator) handlePanelResize(msg ResizeMsg) tea.Cmd {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.width = msg.Width
	e.height = msg.Height
	if e.screen != nil {
		e.screen.Resize(msg.Height, msg.Width)
	}
	if e.pty != nil {
		e.pty.Resize(msg.Height, msg.Width)
	}
	return nil
}

// Focus marks the emulator as focused.
func (e *Emulator) Focus() {
	e.focused = true
}

// Blur marks the emulator as blurred.
func (e *Emulator) Blur() {
	e.focused = false
}

// Close closes the PTY.
func (e *Emulator) Close() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pty != nil {
		e.pty.Close()
		e.pty = nil
	}
}

// Stop terminates the session and switches the panel to the idle ASCII art view.
func (e *Emulator) Stop() {
	e.mu.Lock()
	defer e.mu.Unlock()
	if e.pty != nil {
		e.pty.Close()
		e.pty = nil
	}
	e.stopped = true
}

// SetInitialCWD sets the directory in which the PTY process starts.
func (e *Emulator) SetInitialCWD(dir string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.initialCWD = dir
}

// SetCommandHistory restores a previously saved command history.
func (e *Emulator) SetCommandHistory(history []string) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.commandHistory = history
}

func renderAsciiArt(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	lines := strings.Split(strings.TrimRight(asciiArtIcon, "\n"), "\n")
	artH := len(lines)
	artW := 0
	for _, l := range lines {
		if w := lipgloss.Width(l); w > artW {
			artW = w
		}
	}

	startY := (height - artH) / 2
	if startY < 0 {
		startY = 0
	}
	startX := (width - artW) / 2
	if startX < 0 {
		startX = 0
	}

	var out []string
	for y := 0; y < height; y++ {
		if y >= startY && y-startY < artH {
			line := lines[y-startY]
			pad := startX
			lineW := lipgloss.Width(line)
			if pad+lineW > width {
				line = line[:width-pad]
				lineW = lipgloss.Width(line)
			}
			trail := width - pad - lineW
			if trail < 0 {
				trail = 0
			}
			out = append(out, strings.Repeat(" ", pad)+line+strings.Repeat(" ", trail))
		} else {
			out = append(out, strings.Repeat(" ", width))
		}
	}
	return strings.Join(out, "\n")
}

func emptyView(width, height int) string {
	if width <= 0 || height <= 0 {
		return ""
	}
	line := ""
	for i := 0; i < width; i++ {
		line += " "
	}
	var lines []string
	for i := 0; i < height; i++ {
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

func keyToBytes(msg tea.KeyMsg) []byte {
	switch msg.Type {
	case tea.KeyUp:
		return []byte("\x1b[A")
	case tea.KeyDown:
		return []byte("\x1b[B")
	case tea.KeyRight:
		return []byte("\x1b[C")
	case tea.KeyLeft:
		return []byte("\x1b[D")
	case tea.KeyHome:
		return []byte("\x1b[H")
	case tea.KeyEnd:
		return []byte("\x1b[F")
	case tea.KeyPgUp:
		return []byte("\x1b[5~")
	case tea.KeyPgDown:
		return []byte("\x1b[6~")
	case tea.KeyDelete:
		return []byte("\x1b[3~")
	case tea.KeyInsert:
		return []byte("\x1b[2~")
	case tea.KeyTab:
		return []byte("\t")
	case tea.KeyShiftTab:
		// Standard terminal sequence for Shift+Tab.
		return []byte("\x1b[Z")
	case tea.KeyEnter:
		if msg.String() == "shift+enter" {
			return []byte("\n")
		}
		return []byte("\r")
	case tea.KeyBackspace:
		return []byte("\x7f")
	case tea.KeyEscape:
		return []byte("\x1b")
	case tea.KeySpace:
		return []byte(" ")
	case tea.KeyCtrlC:
		return []byte("\x03")
	case tea.KeyCtrlD:
		return []byte("\x04")
	case tea.KeyCtrlJ:
		// Ctrl+J / LF is what some terminals send for Shift+Enter.
		return []byte("\n")
	case tea.KeyCtrlL:
		return []byte("\x0c")
	case tea.KeyCtrlZ:
		return []byte("\x1a")
	case tea.KeyRunes:
		// Shift+Enter on some terminals is delivered as a literal newline
		// rune rather than a modified KeyEnter.
		if string(msg.Runes) == "\n" {
			return []byte("\n")
		}
		return []byte(string(msg.Runes))
	}
	return nil
}

// CursorBlinkMsg is sent by the host to toggle the cursor blink state.
// A single host-level timer should broadcast this message to all visible
// terminal emulators so their cursors blink in sync.
type CursorBlinkMsg struct{}

// cursorBlinkMsg is the legacy internal name, kept as an alias for compatibility.
type cursorBlinkMsg = CursorBlinkMsg

// PtyReadyMsg is sent when the PTY is ready for listening.
type PtyReadyMsg struct {
	SessionID      string
	AlreadyRunning bool
}

// listenPty returns a command that blocks until PTY output is available.
func listenPty(sessionID string, p *Pty) tea.Cmd {
	return func() tea.Msg {
		select {
		case data := <-p.Output:
			return PtyOutputMsg{SessionID: sessionID, Data: data}
		case err := <-p.Errors:
			return PtyExitMsg{SessionID: sessionID, Err: err}
		}
	}
}
