package cli

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/GiulioSavini/secret-rotator/internal/application"
	"github.com/GiulioSavini/secret-rotator/internal/domain"
	"github.com/GiulioSavini/secret-rotator/internal/infrastructure/audit"
	"github.com/GiulioSavini/secret-rotator/internal/infrastructure/container"
	"github.com/GiulioSavini/secret-rotator/internal/infrastructure/credential"
	"github.com/GiulioSavini/secret-rotator/internal/infrastructure/envstore"
)

// This file is the composition root: the single place where concrete adapters
// are chosen and handed to the use cases. Everywhere else in the CLI works
// with the interfaces the domain declares, so swapping an adapter -- a socket
// proxy for the Docker socket, a different audit backend -- is a change here
// and nowhere else.

// runtime holds the adapters a command needs, plus the cleanup that releases
// them.
type runtime struct {
	rotators  domain.RotatorRegistry
	generator domain.PasswordGenerator
	env       domain.EnvStore
	fleet     domain.ContainerFleet
	audit     domain.AuditTrail
	history   domain.RotationHistory
	reporter  domain.Reporter

	closers []func()
}

// close releases every adapter that holds a resource.
func (r *runtime) close() {
	for i := len(r.closers) - 1; i >= 0; i-- {
		r.closers[i]()
	}
}

// runtimeOptions selects which adapters are built.
type runtimeOptions struct {
	// needsDocker asks for a real container fleet. When false, or when the
	// daemon cannot be reached, a no-op fleet is used instead.
	needsDocker bool
	// passphrase is the master passphrase for the audit trail, already
	// resolved. Empty disables recording.
	passphrase string
	// quiet suppresses step reporting.
	quiet bool
}

// newRuntime assembles the adapters.
func newRuntime(cmd *cobra.Command, opts runtimeOptions) (*runtime, error) {
	rt := &runtime{
		rotators:  credential.NewRegistry(),
		generator: credential.NewGenerator(),
		env:       envstore.NewStore(),
		fleet:     application.NoopFleet(),
		audit:     application.NoopAudit(),
		reporter:  application.NoopReporter(),
	}

	if !opts.quiet {
		rt.reporter = &consoleReporter{out: cmd.ErrOrStderr(), verbose: verboseFlag}
	}

	if opts.needsDocker {
		manager, err := container.NewSDKClient()
		if err != nil {
			return nil, fmt.Errorf("connecting to the Docker daemon: %w", err)
		}
		rt.closers = append(rt.closers, func() { _ = manager.Close() })
		rt.fleet = container.NewFleet(manager, container.DefaultStepTimeout)
	}

	if opts.passphrase != "" {
		history := newHistory(opts.passphrase)
		rt.audit = history
		rt.history = history
	} else {
		fmt.Fprintln(cmd.ErrOrStderr(),
			"warning: no master passphrase (ROTATOR_MASTER_KEY); rotations will not be recorded in the history")
	}

	return rt, nil
}

// newHistory builds the encrypted rotation log, which is both the audit trail
// and the source of truth for how old each credential is.
//
// keepPreviousValue mirrors the historical behaviour of the log. Set
// ROTATOR_AUDIT_OMIT_PREVIOUS=1 to stop archiving replaced credentials, at the
// cost of losing the ability to recover one.
func newHistory(passphrase string) *audit.History {
	store := audit.NewStore(historyPath(), []byte(passphrase))
	trail := audit.NewTrail(store, os.Getenv("ROTATOR_AUDIT_OMIT_PREVIOUS") == "")
	return audit.NewHistory(store, trail)
}

// openHistory returns the rotation history for read-only commands, or nil when
// no passphrase is available. A nil history is meaningful: the compliance check
// reports "unknown" rather than pretending every policy is met.
func openHistory(passphrase string) domain.RotationHistory {
	pp := resolvePassphrase(passphrase)
	if pp == "" {
		return nil
	}
	return newHistory(pp)
}

// complianceUseCase builds the policy check from the assembled adapters.
func (r *runtime) complianceUseCase() *application.CheckCompliance {
	return application.NewCheckCompliance(r.history, application.SystemClock())
}

// rotateUseCase builds the rotation use case from the assembled adapters.
func (r *runtime) rotateUseCase() *application.RotateSecret {
	return application.NewRotateSecret(application.RotateSecretDeps{
		Rotators:  r.rotators,
		Generator: r.generator,
		Env:       r.env,
		Fleet:     r.fleet,
		Audit:     r.audit,
		Clock:     application.SystemClock(),
		Reporter:  r.reporter,
	})
}

// planUseCase builds the read-only planning use case.
func (r *runtime) planUseCase() *application.PlanRotation {
	return application.NewPlanRotation(r.rotators, r.env, r.fleet)
}

// consoleReporter renders use-case progress on stderr, so that stdout stays
// reserved for the command's actual output.
type consoleReporter struct {
	out     interface{ Write([]byte) (int, error) }
	verbose bool
}

func (r *consoleReporter) Step(step domain.Step, message string) {
	if !r.verbose {
		return
	}
	fmt.Fprintf(r.out, "  %-14s %s\n", step, message)
}

func (r *consoleReporter) Warn(message string) {
	fmt.Fprintf(r.out, "warning: %s\n", message)
}
