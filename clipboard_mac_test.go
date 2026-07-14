package portalis

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

// loadImageIntoClipboard loads a PNG file into the macOS clipboard
// using an inline Swift script. Used by the test below.
func loadImageIntoClipboard(t *testing.T, path string) {
	t.Helper()
	if _, err := exec.LookPath("swift"); err != nil {
		t.Skip("swift not available")
	}
	const script = `import Cocoa
import Foundation

let url = URL(fileURLWithPath: CommandLine.arguments[1])
guard let data = try? Data(contentsOf: url) else {
    print("ERR")
    exit(1)
}
let pb = NSPasteboard.general
pb.clearContents()
_ = pb.setData(data, forType: NSPasteboard.PasteboardType("public.png"))
print("OK")
`
	tmp := filepath.Join(os.TempDir(), "loadclip_test.swift")
	if err := os.WriteFile(tmp, []byte(script), 0o644); err != nil {
		t.Fatal(err)
	}
	defer os.Remove(tmp)
	if out, err := exec.Command("swift", tmp, path).CombinedOutput(); err != nil {
		t.Fatalf("loadclip: %v\n%s", err, out)
	}
}

func TestPasteMac_Image(t *testing.T) {
	if _, err := exec.LookPath("swift"); err != nil {
		t.Skip("swift not available; skipping macOS image paste test")
	}
	png := findSamplePNG(t)
	loadImageIntoClipboard(t, png)

	text, imgPath, err := pasteMac()
	if err != nil {
		t.Fatalf("pasteMac: %v", err)
	}
	if imgPath == "" {
		t.Fatalf("pasteMac returned no image path; text=%q", text)
	}
	if !strings.HasSuffix(imgPath, ".png") {
		t.Errorf("imgPath = %q, want .png suffix", imgPath)
	}
	// File must exist and have non-zero size.
	info, err := os.Stat(imgPath)
	if err != nil {
		t.Fatalf("stat %s: %v", imgPath, err)
	}
	if info.Size() == 0 {
		t.Error("image file is empty")
	}
	t.Logf("OK: text=%q imgPath=%s (%d bytes)", text, imgPath, info.Size())
}

func TestPasteMac_Text(t *testing.T) {
	if err := exec.Command("bash", "-c", `echo -n "hello world" | pbcopy`).Run(); err != nil {
		t.Skip("pbcopy not available")
	}
	text, imgPath, err := pasteMac()
	if err != nil {
		t.Fatalf("pasteMac: %v", err)
	}
	if imgPath != "" {
		t.Errorf("unexpected image path: %q", imgPath)
	}
	if text != "hello world" {
		t.Errorf("text = %q, want %q", text, "hello world")
	}
}

func findSamplePNG(t *testing.T) string {
	t.Helper()
	// Look for any cached pi-clipboard PNG or fall back to a generated one.
	for _, dir := range []string{os.TempDir()} {
		matches, _ := filepath.Glob(filepath.Join(dir, "pi-clipboard-*.png"))
		if len(matches) > 0 {
			return matches[0]
		}
	}
	// Generate a minimal 1x1 PNG so the test is self-contained.
	const px = `iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAQAAAC1HAwCAAAAC0lEQVR42mNkYAAAAAYAAjCB0C8AAAAASUVORK5CYII=`
	out := filepath.Join(os.TempDir(), "test-pixel.png")
	if err := os.WriteFile(out, []byte(px), 0o644); err != nil {
		t.Fatal(err)
	}
	return out
}
