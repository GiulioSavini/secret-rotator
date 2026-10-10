package main

import (
	"errors"
	"os"

	"github.com/GiulioSavini/secret-rotator/internal/interfaces/cli"
)

// Exit codes. They are a contract: `rotator status --overdue` is meant to be
// wired into monitoring, which has to tell "the check found something" apart
// from "the tool broke".
const (
	exitFailure = 1
	exitOverdue = 2
)

func main() {
	err := cli.NewRootCmd().Execute()

	switch {
	case err == nil:
		return
	case errors.Is(err, cli.ErrOverdue):
		// The command already printed what is overdue; this is a finding,
		// not a crash, so it gets its own code and no error banner.
		os.Exit(exitOverdue)
	default:
		os.Exit(exitFailure)
	}
}
