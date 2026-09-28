package main

import (
	"os"

	"github.com/rithikamandiv-ux/stackrun/internal/cli"
)

func main() {
	if err := cli.Execute(); err != nil {
		os.Exit(1)
	}
}
