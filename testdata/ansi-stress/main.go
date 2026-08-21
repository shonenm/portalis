package main

import (
	"fmt"
	"os"
	"time"

	tea "github.com/charmbracelet/bubbletea"

	"github.com/shonenm/portalis"
	"github.com/shonenm/portalis/internal/stressfixture"
)

type frameMsg struct{}

type model struct {
	screen    *portalis.Screen
	parser    *portalis.Parser
	started   bool
	done      bool
	frame     int
	startedAt time.Time
	maxFeed   time.Duration
}

func newModel() model {
	screen := portalis.NewScreen(stressfixture.Rows, stressfixture.Cols)
	return model{
		screen: screen,
		parser: portalis.NewParser(screen),
	}
}

func (model) Init() tea.Cmd {
	return nil
}

func (m model) Update(message tea.Msg) (tea.Model, tea.Cmd) {
	switch message := message.(type) {
	case tea.KeyMsg:
		switch message.String() {
		case "ctrl+c", "q":
			return m, tea.Quit
		case "enter":
			if !m.started {
				m.started = true
				m.startedAt = time.Now()
				return m, nextFrame()
			}
		}
	case tea.WindowSizeMsg:
		m.screen.Resize(stressfixture.Rows, stressfixture.Cols)
	case frameMsg:
		if m.done {
			return m, nil
		}
		m.frame++
		started := time.Now()
		stressfixture.FeedChunks(
			stressfixture.Frame(m.frame),
			[]int{1, 2, 3, 5, 8, 13, 21, 34},
			m.parser.Feed,
		)
		if elapsed := time.Since(started); elapsed > m.maxFeed {
			m.maxFeed = elapsed
		}
		if m.frame < stressfixture.FrameCount {
			return m, nextFrame()
		}

		m.done = true
		total := time.Since(m.startedAt)
		m.parser.Feed([]byte(fmt.Sprintf(
			"\x1b[24;1HMAX FEED %s | TOTAL %s",
			m.maxFeed.Round(time.Microsecond),
			total.Round(time.Millisecond),
		)))
	}
	return m, nil
}

func (m model) View() string {
	if !m.started {
		return "PORTALIS ANSI STRESS READY\n\nPress Enter to run 400 complex frames\n\nCtrl+C or q quits"
	}
	return m.screen.Render()
}

func nextFrame() tea.Cmd {
	return func() tea.Msg {
		return frameMsg{}
	}
}

func main() {
	program := tea.NewProgram(newModel(), tea.WithAltScreen())
	if _, err := program.Run(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
