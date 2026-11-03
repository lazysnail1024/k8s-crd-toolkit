package main

import (
	"os"
	"strconv"
	"strings"
)

func rss() int64 {
	b, e := os.ReadFile("/proc/self/status")
	if e != nil {
		return -1
	}
	for _, line := range strings.Split(string(b), "\n") {
		if strings.HasPrefix(line, "VmRSS:") {
			f := strings.Fields(line)
			n, _ := strconv.ParseInt(f[1], 10, 64)
			return n * 1024
		}
	}
	return -1
}
