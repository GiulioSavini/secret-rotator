package main

import (
	"os"

	"github.com/giulio/secret-rotator/internal/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
