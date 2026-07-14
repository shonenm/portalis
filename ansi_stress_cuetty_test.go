//go:build cuetty || livepi

package portalis_test

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

const cueTTYSession = "ansi-stress"

func TestANSIStressCueTTY(t *testing.T) {
	root := projectRoot(t)
	binary := filepath.Join(t.TempDir(), "portalis-ansi-stress")
	build := exec.Command("go", "build", "-o", binary, "./testdata/ansi-stress")
	build.Dir = root
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build ANSI stress fixture: %v\n%s", err, output)
	}

	artifactRoot := filepath.Join(root, "cuetty-artifacts", cueTTYSession)
	if err := os.RemoveAll(artifactRoot); err != nil {
		t.Fatalf("remove old artifacts: %v", err)
	}
	cueTTYIgnore(root, cueTTYSession, "exit")
	t.Cleanup(func() { cueTTYIgnore(root, cueTTYSession, "exit") })

	events := []string{
		"0000ms launch fixture",
		"0100ms resize terminal to 100x30",
		"0200ms capture before state",
		"0300ms press Enter",
		"2300ms assert final state",
		"2400ms capture after state",
	}

	cueTTY(t, root, cueTTYSession, "launch", binary)
	cueTTY(t, root, cueTTYSession, "resize", "100", "30")
	cueTTY(t, root, cueTTYSession, "wait", "200")
	cueTTYExpect(t, root, cueTTYSession, "contain", "PORTALIS ANSI STRESS READY")
	cueTTY(t, root, cueTTYSession, "save", "before")

	cueTTY(t, root, cueTTYSession, "press", "Enter")
	cueTTY(t, root, cueTTYSession, "wait", "2000")
	cueTTYExpect(t, root, cueTTYSession, "contain", "STRESS PASS")
	cueTTYExpect(t, root, cueTTYSession, "contain", "FRAME 0400")
	cueTTYExpect(t, root, cueTTYSession, "not", "FRAME 0399")
	cueTTYExpect(t, root, cueTTYSession, "not", "CORRUPTION")

	text := cueTTY(t, root, cueTTYSession, "text")
	if count := strings.Count(text, "FRAME "); count != 1 {
		t.Fatalf("visible FRAME count = %d, want 1\n%s", count, text)
	}
	if !strings.Contains(text, "MAX FEED") || !strings.Contains(text, "TOTAL") {
		t.Fatalf("missing timing diagnostics\n%s", text)
	}
	cueTTY(t, root, cueTTYSession, "save", "after")

	writeExpressiveArtifacts(t, artifactRoot, events, text)
}

func projectRoot(t *testing.T) string {
	t.Helper()
	root, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	return root
}

func cueTTY(t *testing.T, dir, session string, arguments ...string) string {
	t.Helper()
	commandArguments := append([]string{"-s", session}, arguments...)
	command := exec.Command("cuetty-cli", commandArguments...)
	command.Dir = dir
	output, err := command.CombinedOutput()
	if err != nil {
		t.Fatalf("cuetty-cli %s: %v\n%s", strings.Join(arguments, " "), err, output)
	}
	return string(output)
}

func cueTTYIgnore(dir, session string, arguments ...string) {
	commandArguments := append([]string{"-s", session}, arguments...)
	command := exec.Command("cuetty-cli", commandArguments...)
	command.Dir = dir
	_ = command.Run()
}

func cueTTYExpect(t *testing.T, dir, session, condition, value string) {
	t.Helper()
	output := cueTTY(t, dir, session, "expect", condition, value)
	if !strings.Contains(output, "✅") {
		t.Fatalf("cue-tty expectation did not pass: %s %q\n%s", condition, value, output)
	}
}

func writeExpressiveArtifacts(t *testing.T, artifactRoot string, events []string, text string) {
	t.Helper()
	if err := os.MkdirAll(artifactRoot, 0o755); err != nil {
		t.Fatal(err)
	}

	files := map[string]string{
		"events.log":  strings.Join(events, "\n") + "\n",
		"timeline.md": "# Временная шкала ANSI stress\n\n" + strings.Join(events, "\n\n") + "\n",
		"report.md": fmt.Sprintf(`# Результат теста: ANSI stress через cue-tty

## Статус

пройден

## Намерение

Проверить 400 сложных ANSI/Unicode-кадров в настоящем PTY размером 100×30.

## Сценарий

Дано Portalis stress fixture готово.  
Когда cue-tty нажимает Enter.  
Тогда виден только FRAME 0400, присутствует STRESS PASS и отсутствуют следы предыдущего кадра.

## Фактически

%s

## Артефакты

- before/render.txt
- before/state.json
- after/render.txt
- after/state.json
- events.log
- timeline.md
`, text),
	}
	for name, content := range files {
		if err := os.WriteFile(filepath.Join(artifactRoot, name), []byte(content), 0o644); err != nil {
			t.Fatalf("write %s: %v", name, err)
		}
	}
}
