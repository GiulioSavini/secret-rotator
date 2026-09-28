package application

import (
	"context"
	"errors"
	"fmt"

	"github.com/giulio/secret-rotator/internal/domain"
)

// RotateSecret is the use case behind `rotator rotate` and every scheduled
// rotation.
//
// It drives the pipeline and, on any failure after the first mutation, runs
// the compensation plan the domain derives from what actually happened. It
// holds no state between calls, so one instance serves the daemon.
type RotateSecret struct {
	rotators  domain.RotatorRegistry
	generator domain.PasswordGenerator
	env       domain.EnvStore
	fleet     domain.ContainerFleet
	audit     domain.AuditTrail
	clock     domain.Clock
	reporter  domain.Reporter
}

// RotateSecretDeps collects the ports the use case needs. Audit and reporter
// are optional: a rotation without a passphrase records nothing, and a
// rotation nobody is watching reports nothing.
type RotateSecretDeps struct {
	Rotators  domain.RotatorRegistry
	Generator domain.PasswordGenerator
	Env       domain.EnvStore
	Fleet     domain.ContainerFleet
	Audit     domain.AuditTrail
	Clock     domain.Clock
	Reporter  domain.Reporter
}

// NewRotateSecret builds the use case, defaulting the optional ports to no-ops
// so the pipeline never has to nil-check them.
func NewRotateSecret(deps RotateSecretDeps) *RotateSecret {
	if deps.Audit == nil {
		deps.Audit = noopAudit{}
	}
	if deps.Reporter == nil {
		deps.Reporter = noopReporter{}
	}
	if deps.Clock == nil {
		deps.Clock = systemClock{}
	}
	return &RotateSecret{
		rotators:  deps.Rotators,
		generator: deps.Generator,
		env:       deps.Env,
		fleet:     deps.Fleet,
		audit:     deps.Audit,
		clock:     deps.Clock,
		reporter:  deps.Reporter,
	}
}

// Execute rotates one secret.
//
// The order is deliberate: the backing service is changed and verified before
// any file is touched, so a service that refuses the new credential costs
// nothing; the files are updated next; the containers restart last.
func (uc *RotateSecret) Execute(ctx context.Context, secret *domain.Secret) error {
	rotator, err := uc.rotators.For(secret.Kind())
	if err != nil {
		return err
	}

	// Resolve container references before mutating anything, so a typo in
	// `containers:` fails while the system is still untouched.
	containers, err := uc.fleet.Resolve(ctx, secret.Containers())
	if err != nil {
		return fmt.Errorf("resolving containers for secret %s: %w", secret.Name(), err)
	}

	rotation := domain.BeginRotation(secret, uc.clock.Now())

	primary := secret.PrimaryEnvFile()
	doc, err := uc.env.Open(primary)
	if err != nil {
		return fmt.Errorf("reading %s: %w", primary, err)
	}

	currentRaw, found := doc.Lookup(secret.EnvKey())
	if !found {
		return fmt.Errorf("%w: %s is not present in %s",
			domain.ErrSecretKeyMissing, secret.EnvKey(), primary)
	}
	rotation.RecordCurrentValue(domain.NewCredential(currentRaw), doc.Snapshot())
	uc.reporter.Step(domain.StepRead, fmt.Sprintf("read %s from %s", secret.EnvKey(), primary))

	// The admin credential may be the secret being rotated, in which case the
	// file holds the current value and the process environment holds a stale
	// one. Resolving from the document first is what makes repeat rotations
	// work for a long-running daemon.
	target := uc.resolveAdminPassword(secret, doc, rotation.Previous())

	next, err := uc.generator.Generate(secret.Length())
	if err != nil {
		return uc.fail(ctx, rotation, fmt.Errorf("generating a new value: %w", err))
	}
	rotation.RecordGenerated(next)
	uc.reporter.Step(domain.StepGenerated, "generated a new value")

	if secret.Kind().HasBackingService() {
		if err := rotator.Apply(ctx, target, next); err != nil {
			return uc.compensate(ctx, rotation, rotator, target,
				fmt.Errorf("applying the new credential: %w", err))
		}
		rotation.RecordApplied()
		uc.reporter.Step(domain.StepApplied, fmt.Sprintf("applied to %s", target.Addr(secret.Kind())))
	}

	if secret.NeedsVerification() {
		if err := rotator.Verify(ctx, target, next); err != nil {
			return uc.compensate(ctx, rotation, rotator, target,
				fmt.Errorf("verifying the new credential: %w", err))
		}
		rotation.RecordVerified()
		uc.reporter.Step(domain.StepVerified, "verified the new credential")
	}

	for _, path := range secret.EnvFiles() {
		before, err := uc.env.Write(path, secret.EnvKey(), next)
		if err != nil {
			return uc.compensate(ctx, rotation, rotator, target,
				fmt.Errorf("writing %s: %w", path, err))
		}
		rotation.RecordFileWritten(path, before)
		uc.reporter.Step(domain.StepFilesUpdated, fmt.Sprintf("updated %s", path))
	}

	if len(containers) > 0 {
		if err := uc.fleet.Restart(ctx, containers); err != nil {
			// Record the restart before compensating: containers that did
			// come back up have to be restarted again with the old values.
			rotation.RecordRestarted(containers)
			return uc.compensate(ctx, rotation, rotator, target,
				fmt.Errorf("restarting containers: %w", err))
		}
		rotation.RecordRestarted(containers)
		uc.reporter.Step(domain.StepRestarted, fmt.Sprintf("restarted %d container(s)", len(containers)))
	}

	rotation.RecordDone()
	if err := uc.audit.Record(ctx, rotation.Succeeded(uc.clock.Now())); err != nil {
		uc.reporter.Warn(fmt.Sprintf("rotation succeeded but was not recorded: %v", err))
	}
	return nil
}

// resolveAdminPassword determines the credential used to connect to the
// backing service.
//
// Three sources, in order:
//
//  1. an explicit provider.password;
//  2. the .env entry named by provider.password_env -- read from the document
//     rather than the process environment, because a daemon keeps the
//     environment it started with and only the file holds the current value
//     once the admin credential has itself been rotated once;
//  3. the secret's own current value, for the common case of an account
//     rotating its own password. This is what makes a Redis entry work with
//     nothing but a host, since requirepass is both the credential being
//     rotated and the one used to connect.
//
// Step 3 is skipped when a different account is being targeted, where the
// secret's value says nothing about the administrative credential.
func (uc *RotateSecret) resolveAdminPassword(
	secret *domain.Secret, doc domain.EnvDocument, current domain.Credential,
) domain.Target {
	target := secret.Target()
	if !target.AdminPassword.IsZero() {
		return target
	}

	if target.AdminPasswordEnv != "" {
		if key, err := domain.NewEnvKey(target.AdminPasswordEnv); err == nil {
			if raw, ok := doc.Lookup(key); ok && raw != "" {
				return target.WithAdminPassword(domain.NewCredential(raw))
			}
		}
		return target
	}

	if target.TargetUser == "" || target.TargetUser == target.AdminUser {
		return target.WithAdminPassword(current)
	}
	return target
}

// compensate runs the undo plan for a failed rotation and reports the outcome.
func (uc *RotateSecret) compensate(
	ctx context.Context,
	rotation *domain.Rotation,
	rotator domain.CredentialRotator,
	target domain.Target,
	cause error,
) error {
	var failures []error

	for _, action := range rotation.CompensationPlan() {
		switch action.Type {
		case domain.CompensationRestoreFile:
			if err := uc.env.Restore(action.Path, action.Snapshot); err != nil {
				failures = append(failures, fmt.Errorf("restoring %s: %w", action.Path, err))
			}
		case domain.CompensationRestartContainers:
			if err := uc.fleet.Restart(ctx, action.Containers); err != nil {
				failures = append(failures, fmt.Errorf("restarting containers: %w", err))
			}
		case domain.CompensationRestoreCredential:
			if err := rotator.Restore(ctx, target, rotation.Previous(), rotation.Next()); err != nil {
				failures = append(failures, fmt.Errorf("restoring the previous credential: %w", err))
			}
		}
	}

	if err := uc.audit.Record(ctx, rotation.Failed(uc.clock.Now(), cause)); err != nil {
		uc.reporter.Warn(fmt.Sprintf("rotation failed and the failure was not recorded: %v", err))
	}

	if len(failures) == 0 {
		return fmt.Errorf("%w: %w (rolled back)", domain.ErrRotationFailed, cause)
	}
	return fmt.Errorf("%w: %w; %w: %s. %s",
		domain.ErrRotationFailed, cause,
		domain.ErrRollbackFailed, errors.Join(failures...),
		rotation.StateDescription(failures))
}

// fail records a failure that happened before anything was mutated.
func (uc *RotateSecret) fail(ctx context.Context, rotation *domain.Rotation, cause error) error {
	if err := uc.audit.Record(ctx, rotation.Failed(uc.clock.Now(), cause)); err != nil {
		uc.reporter.Warn(fmt.Sprintf("rotation failed and the failure was not recorded: %v", err))
	}
	return fmt.Errorf("%w: %w", domain.ErrRotationFailed, cause)
}
