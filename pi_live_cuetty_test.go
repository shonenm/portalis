//go:build livepi

package portalis_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

const (
	piLiveCols           = 120
	piLiveRows           = 40
	piMarkerTimeout      = 120 * time.Second
	maxScreenReadLatency = 750 * time.Millisecond
)

type liveObservation struct {
	Marker          string
	Polls           int
	MaxWorkingLines int
	MaxReadLatency  time.Duration
	Elapsed         time.Duration
	FinalScreen     string
}

func TestPiLiveCueTTY(t *testing.T) {
	root := projectRoot(t)
	binary := filepath.Join(t.TempDir(), "portalis-pi-live")
	build := exec.Command("go", "build", "-o", binary, "./testdata/pi-live")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build Pi live fixture: %v\n%s", err, output)
	}

	scenarios := []struct {
		name        string
		session     string
		cols        int
		rows        int
		tmux        bool
		tmuxSession string
	}{
		{name: "direct-120", session: "pi-live", cols: 120, rows: 40},
		{name: "direct-80", session: "pi-live-80", cols: 80, rows: 40},
		{name: "tmux", session: "pi-tmux-live", cols: 120, rows: 40, tmux: true, tmuxSession: "portalis-pi-tmux-live"},
	}

	for _, scenario := range scenarios {
		scenario := scenario
		t.Run(scenario.name, func(t *testing.T) {
			runPiLiveScenario(t, root, binary, scenario.session, scenario.cols, scenario.rows, scenario.tmux, scenario.tmuxSession)
		})
	}
}

func runPiLiveScenario(t *testing.T, root, binary, session string, cols, rows int, useTmux bool, tmuxSession string) {
	t.Helper()

	artifactRoot := filepath.Join(root, "cuetty-artifacts", session)
	if err := os.RemoveAll(artifactRoot); err != nil {
		t.Fatalf("remove old artifacts: %v", err)
	}
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatalf("create artifact directory: %v", err)
	}
	cueTTYIgnore(root, session, "exit")
	killTmuxSession(tmuxSession)
	t.Cleanup(func() {
		cueTTYIgnore(root, session, "exit")
		killTmuxSession(tmuxSession)
	})

	traceBase := filepath.Join(artifactRoot, "raw-pty")
	viewHistory := filepath.Join(artifactRoot, "view-history.log")
	launchArguments := []string{
		"launch",
		"/usr/bin/env",
		"PORTALIS_RAW_TRACE=" + traceBase,
		"PORTALIS_VIEW_HISTORY=" + viewHistory,
		binary,
	}
	if useTmux {
		launchArguments = append(launchArguments, "--tmux", "--tmux-session", tmuxSession)
	}
	cueTTY(t, root, session, launchArguments...)
	cueTTY(t, root, session, "resize", fmt.Sprint(cols), fmt.Sprint(rows))
	waitForScreenText(t, root, session, "qwen-3.5-9b", 30*time.Second)
	cueTTY(t, root, session, "save", "before")

	firstPrompt := "Reply without tools. Write 40 numbered lines with ASCII, CJK, emoji, combining marks, and code. Finish with these ASCII fragments adjacent, without whitespace or punctuation: PZ9Q7 then END42."
	cueTTY(t, root, session, "type", firstPrompt)
	cueTTY(t, root, session, "press", "Enter")
	first := observePiStream(t, root, session, artifactRoot, "PZ9Q7END42")
	cueTTY(t, root, session, "save", "after-first")

	cueTTY(t, root, session, "wait", "300")
	secondPrompt := "Reply without tools. Produce a 25-row markdown table with mixed-width Unicode. Finish with these ASCII fragments adjacent, without whitespace or punctuation: RX8M3 then END77."
	cueTTY(t, root, session, "type", secondPrompt)
	cueTTY(t, root, session, "press", "Enter")
	second := observePiStream(t, root, session, artifactRoot, "RX8M3END77")
	cueTTY(t, root, session, "save", "after-second")

	writePiLiveArtifacts(t, artifactRoot, useTmux, first, second)
}

func observePiStream(t *testing.T, root, session, artifactRoot, marker string) liveObservation {
	t.Helper()

	started := time.Now()
	observation := liveObservation{Marker: marker}
	firstSnapshotSaved := false
	for time.Since(started) < piMarkerTimeout {
		readStarted := time.Now()
		screen := cueTTY(t, root, session, "text")
		readLatency := time.Since(readStarted)
		observation.Polls++
		if readLatency > observation.MaxReadLatency {
			observation.MaxReadLatency = readLatency
		}

		workingLines := 0
		for _, line := range strings.Split(screen, "\n") {
			if strings.Contains(line, "Working...") {
				workingLines++
			}
		}
		if workingLines > observation.MaxWorkingLines {
			observation.MaxWorkingLines = workingLines
		}
		if workingLines > 1 {
			saveLiveSnapshot(t, artifactRoot, marker+"-failure.render.txt", screen)
			t.Fatalf("%s: %d simultaneous Working... lines\n%s", marker, workingLines, screen)
		}

		if !firstSnapshotSaved && strings.Contains(screen, "Working...") {
			saveLiveSnapshot(t, artifactRoot, marker+"-streaming-first.render.txt", screen)
			firstSnapshotSaved = true
		}
		saveLiveSnapshot(t, artifactRoot, marker+"-streaming-last.render.txt", screen)

		if strings.Contains(screen, marker) && workingLines == 0 {
			observation.Elapsed = time.Since(started)
			observation.FinalScreen = screen
			if observation.MaxReadLatency > maxScreenReadLatency {
				t.Fatalf(
					"%s: max screen read latency %s exceeds %s",
					marker,
					observation.MaxReadLatency,
					maxScreenReadLatency,
				)
			}
			return observation
		}
		time.Sleep(150 * time.Millisecond)
	}

	observation.Elapsed = time.Since(started)
	observation.FinalScreen = cueTTY(t, root, session, "text")
	t.Fatalf("timeout waiting for %s after %s\n%s", marker, observation.Elapsed, observation.FinalScreen)
	return observation
}

func waitForScreenText(t *testing.T, root, session, expected string, timeout time.Duration) {
	t.Helper()
	started := time.Now()
	for time.Since(started) < timeout {
		screen := cueTTY(t, root, session, "text")
		if strings.Contains(screen, expected) {
			return
		}
		time.Sleep(150 * time.Millisecond)
	}
	t.Fatalf("timeout waiting for %q\n%s", expected, cueTTY(t, root, session, "text"))
}

func saveLiveSnapshot(t *testing.T, artifactRoot, name, screen string) {
	t.Helper()
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(artifactRoot, name), []byte(screen), 0o644); err != nil {
		t.Fatal(err)
	}
}

func writePiLiveArtifacts(
	t *testing.T,
	artifactRoot string,
	useTmux bool,
	first liveObservation,
	second liveObservation,
) {
	t.Helper()
	mode := "direct Pi"
	if useTmux {
		mode = "tmux → Pi"
	}
	events := fmt.Sprintf(
		"mode=%s\nfirst polls=%d elapsed=%s max_read=%s max_working=%d\nsecond polls=%d elapsed=%s max_read=%s max_working=%d\n",
		mode,
		first.Polls,
		first.Elapsed,
		first.MaxReadLatency,
		first.MaxWorkingLines,
		second.Polls,
		second.Elapsed,
		second.MaxReadLatency,
		second.MaxWorkingLines,
	)
	report := fmt.Sprintf(`# Результат live Pi E2E

## Статус

пройден

## Тракт

cue-tty → Portalis → %s

## Модель

home-pc/qwen-3.5-9b, --no-session

## Первый промпт

- marker: %s
- elapsed: %s
- max screen read: %s
- max Working lines: %d

## Второй промпт

- marker: %s
- elapsed: %s
- max screen read: %s
- max Working lines: %d
`, mode, first.Marker, first.Elapsed, first.MaxReadLatency, first.MaxWorkingLines, second.Marker, second.Elapsed, second.MaxReadLatency, second.MaxWorkingLines)

	saveLiveSnapshot(t, artifactRoot, "events.log", events)
	saveLiveSnapshot(t, artifactRoot, "timeline.md", "# Timeline\n\n```text\n"+events+"```\n")
	saveLiveSnapshot(t, artifactRoot, "report.md", report)
}

func killTmuxSession(session string) {
	if session == "" {
		return
	}
	command := exec.Command("tmux", "kill-session", "-t", session)
	_ = command.Run()
}
