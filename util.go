package main

import (
	"os"
	"fmt"
	"os/exec"
)

// currently works on linux only
// on other OS 80 is assumed
func getTerminalSize() int {
	cmd := exec.Command("stty", "size")
	cmd.Stdin = os.Stdin
	out, err1 := cmd.Output()
	res := string(out)
	var rows, cols int;
	_, err2 := fmt.Sscanf(res, "%d %d", &rows, &cols);
	if err1 != nil || err2 != nil || cols == 0 {
		return 80
	}
	return cols
}

