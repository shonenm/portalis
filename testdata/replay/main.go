package main

import (
	"bufio"
	"fmt"
	"os"
	"strconv"
	"strings"

	"github.com/shonenm/portalis"
)

func main() {
	if len(os.Args) != 3 {
		fmt.Fprintln(os.Stderr, "usage: replay raw-pty chunks")
		os.Exit(2)
	}
	raw, err := os.ReadFile(os.Args[1])
	if err != nil {
		panic(err)
	}
	chunksFile, err := os.Open(os.Args[2])
	if err != nil {
		panic(err)
	}
	defer chunksFile.Close()

	emulator := portalis.NewEmulator("replay", "Replay", "sleep", []string{"1000"})
	if err := emulator.StartSync(nil); err != nil {
		panic(err)
	}
	emulator.Close()
	emulator.Update(portalis.ResizeMsg{Width: 120, Height: 40})

	scanner := bufio.NewScanner(chunksFile)
	viewIdx := 0
	offset := 0
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}
		n, err := strconv.Atoi(line)
		if err != nil {
			panic(err)
		}
		if offset+n > len(raw) {
			panic("chunk exceeds raw data")
		}
		emulator.Update(portalis.PtyOutputMsg{SessionID: "replay", Data: raw[offset : offset+n]})
		offset += n

		view := emulator.View(120, 40)
		working := 0
		for _, line := range strings.Split(view, "\n") {
			if strings.Contains(line, "Working...") {
				working++
			}
		}
		if working > 0 {
			fmt.Printf("view=%d working=%d\n", viewIdx, working)
		}
		viewIdx++
	}
	if err := scanner.Err(); err != nil {
		panic(err)
	}

	final := emulator.View(120, 40)
	fmt.Println("=== FINAL ===")
	fmt.Println(final)
}
