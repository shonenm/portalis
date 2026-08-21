package main

import (
	"flag"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"sync"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shonenm/portalis"
)

const modelName = "home-pc/qwen-3.5-9b"
const viewHistorySize = 200

type viewFrame struct {
	Index   int
	Working int
	Invalid int
	Content string
}

var (
	viewHistoryMu sync.Mutex
	viewHistory   [viewHistorySize]viewFrame
	viewIndex     int
	viewSeq       int
)

type model struct {
	emulator *portalis.Emulator
	width    int
	height   int
}

func newModel(useTmux bool, tmuxSession string) (model, error) {
	piPath, err := exec.LookPath("pi")
	if err != nil {
		return model{}, fmt.Errorf("find pi: %w", err)
	}

	command := piPath
	arguments := []string{"--no-session", "--model", modelName}
	if useTmux {
		tmuxPath, tmuxErr := exec.LookPath("tmux")
		if tmuxErr != nil {
			return model{}, fmt.Errorf("find tmux: %w", tmuxErr)
		}
		command = tmuxPath
		arguments = []string{
			"new-session",
			"-s",
			tmuxSession,
			fmt.Sprintf("exec %s --no-session --model %s", piPath, modelName),
		}
	}

	emulator := portalis.NewEmulator("pi-live", "Pi Live", command, arguments)
	emulator.Focus()
	emulator.Update(portalis.ResizeMsg{Width: 120, Height: 40})
	if err := emulator.StartSync([]string{"PI_SKIP_VERSION_CHECK=1"}); err != nil {
		return model{}, fmt.Errorf("start pi: %w", err)
	}
	return model{emulator: emulator, width: 120, height: 40}, nil
}

func (m model) Init() tea.Cmd {
	return m.emulator.Listen()
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	if size, ok := message.(tea.WindowSizeMsg); ok {
		m.width = size.Width
		m.height = size.Height
		return m, m.emulator.Update(portalis.ResizeMsg{Width: size.Width, Height: size.Height})
	}
	return m, m.emulator.Update(message)
}

func (m model) View() string {
	render := m.emulator.View(m.width, m.height)
	recordView(render, m.width, m.height)
	return render
}

func recordView(render string, width, height int) {
	lines := strings.Split(render, "\n")
	working := 0
	invalid := 0
	for _, line := range lines {
		if strings.Contains(line, "Working...") {
			working++
		}
	}
	viewHistoryMu.Lock()
	defer viewHistoryMu.Unlock()
	viewHistory[viewIndex] = viewFrame{Index: viewSeq, Working: working, Invalid: invalid, Content: render}
	viewIndex = (viewIndex + 1) % viewHistorySize
	viewSeq++
	if working > 1 && os.Getenv("PORTALIS_VIEW_HISTORY") != "" {
		dumpViewHistoryUnlocked(os.Getenv("PORTALIS_VIEW_HISTORY") + ".multi-working")
	}
}

func dumpViewHistoryUnlocked(path string) {
	file, err := os.Create(path)
	if err != nil {
		return
	}
	defer file.Close()
	start := viewIndex
	if viewSeq < viewHistorySize {
		start = 0
	}
	count := viewHistorySize
	if viewSeq < viewHistorySize {
		count = viewSeq
	}
	for i := 0; i < count; i++ {
		idx := (start + i) % viewHistorySize
		frame := viewHistory[idx]
		_, _ = fmt.Fprintf(file, "=== VIEW %d working=%d invalid=%d ===\n%s\n", frame.Index, frame.Working, frame.Invalid, frame.Content)
	}
}

func main() {
	useTmux := flag.Bool("tmux", false, "run Pi inside tmux")
	tmuxSession := flag.String("tmux-session", "portalis-pi-live", "tmux session name")
	flag.Parse()

	initial, err := newModel(*useTmux, *tmuxSession)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
	defer func() {
		initial.emulator.Close()
	}()

	program := tea.NewProgram(initial, tea.WithAltScreen(), tea.WithMouseCellMotion())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
