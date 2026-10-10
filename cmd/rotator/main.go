package main

import (
	"os"

	"github.com/GiulioSavini/secret-rotator/internal/interfaces/cli"
)

func main() {
	if err := cli.NewRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}
