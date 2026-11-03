package main

import (
	"os"
	"os/exec"
	"strconv"
	"strings"
)

// macOS ps reports this process's current resident set in KiB, not peak RSS.
func rss() int64 {
	b, e := exec.Command("ps", "-o", "rss=", "-p", strconv.Itoa(os.Getpid())).Output()
	if e != nil {
		return -1
	}
	n, e := strconv.ParseInt(strings.TrimSpace(string(b)), 10, 64)
	if e != nil {
		return -1
	}
	return n * 1024
}
