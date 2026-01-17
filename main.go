package main

import (
	"os"

	"github.com/jankremlacek/monitor/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
