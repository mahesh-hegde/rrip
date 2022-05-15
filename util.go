package main

import (
	"fmt"
	"os"
	"os/exec"
	"strconv"
)

// currently works on linux only
// on other OS 80 is assumed
func getTerminalSize() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err1 := cmd.Output()
	res := string(out)
	var rows, cols int
	_, err2 := fmt.Sscanf(res, "%d %d", &rows, &cols)
	if err1 != nil || err2 != nil || cols == 0 {
		return 80
	}
	return cols
}

func coalesce(a, b string) string {
	if a == "" {
		return b
	}
	return a
}

func quote(s string) string {
	return strconv.Quote(s)
}

func fatal(val ...interface{}) {
	fmt.Fprintln(os.Stderr, val...)
	os.Exit(1)
}

func eprintln(vals ...interface{}) (int, error) {
	return fmt.Fprintln(os.Stderr, vals...)
}

func eprintf(format string, vals ...interface{}) (int, error) {
	return fmt.Fprintf(os.Stderr, format, vals...)
}

func eprint(vals ...interface{}) (int, error) {
	return fmt.Fprint(os.Stderr, vals...)
}

func max(a, b int) int {
	if a > b {
		return a
	}
	return b
}

func check(e error, extra ...interface{}) {
	if e != nil {
		fmt.Fprintln(os.Stderr, extra...)
		fatal(e.Error())
	}
}

func log(vals ...interface{}) {
	if options.Debug {
		fmt.Fprintln(os.Stderr, vals...)
	}
}

func size(bytes int64) string {
	sizes := []int64{1000 * 1000 * 1000, 1000 * 1000, 1000}
	names := []string{"GB", "MB", "KB"}

	if bytes == -1 {
		return "Unknown length"
	}

	for i, sz := range sizes {
		if bytes > sz {
			units := float64(bytes) / float64(sz)
			return strconv.FormatFloat(units, 'f', 1, 64) + names[i]
		}
	}
	return strconv.FormatInt(bytes, 10) + "B"
}

