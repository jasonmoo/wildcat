package main

import (
	"os"

	"github.com/jasonmoo/wildcat/cmd"
)

func main() {
	if err := cmd.Execute(); err != nil {
		os.Exit(1)
	}
}
