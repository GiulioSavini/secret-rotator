package application

import (
	"context"
	"fmt"

	"github.com/giulio/secret-rotator/internal/domain"
)

// PlanRotation answers "what would happen if I rotated this?" without
// touching anything.
//
// Dry run used to be a boolean inside the rotation pipeline, which meant the
// read-only path and the mutating path shared a function and could drift. Here
// it is a separate use case returning a value the caller renders.
type PlanRotation struct {
	rotators domain.RotatorRegistry
	env      domain.EnvStore
	fleet    domain.ContainerFleet
}

// NewPlanRotation builds the planning use case.
func NewPlanRotation(rotators domain.RotatorRegistry, env domain.EnvStore, fleet domain.ContainerFleet) *PlanRotation {
	return &PlanRotation{rotators: rotators, env: env, fleet: fleet}
}

// PlannedAction is one step the rotation would take.
type PlannedAction struct {
	Step        domain.Step
	Description string
}

// RotationPlan is the full set of actions, plus the facts they were derived
// from, so the caller can show both.
type RotationPlan struct {
	Secret     *domain.Secret
	Containers []domain.ContainerRef
	Actions    []PlannedAction
}

// Build produces the plan, verifying only what can be checked read-only: that
// the secret's kind has a rotator, that the primary file exists and holds the
// key, and that every container reference resolves.
func (uc *PlanRotation) Build(ctx context.Context, secret *domain.Secret) (*RotationPlan, error) {
	if _, err := uc.rotators.For(secret.Kind()); err != nil {
		return nil, err
	}

	primary := secret.PrimaryEnvFile()
	doc, err := uc.env.Open(primary)
	if err != nil {
		return nil, fmt.Errorf("reading %s: %w", primary, err)
	}
	if _, found := doc.Lookup(secret.EnvKey()); !found {
		return nil, fmt.Errorf("%w: %s is not present in %s",
			domain.ErrSecretKeyMissing, secret.EnvKey(), primary)
	}

	containers, err := uc.fleet.Resolve(ctx, secret.Containers())
	if err != nil {
		return nil, fmt.Errorf("resolving containers for secret %s: %w", secret.Name(), err)
	}

	plan := &RotationPlan{Secret: secret, Containers: containers}

	plan.add(domain.StepRead, fmt.Sprintf("read the current %s from %s", secret.EnvKey(), primary))
	plan.add(domain.StepGenerated, fmt.Sprintf("generate a new %d-byte value", secret.Length().Int()))

	if secret.Kind().HasBackingService() {
		target := secret.Target()
		plan.add(domain.StepApplied, fmt.Sprintf("set the password of %q on %s (%s)",
			target.EffectiveTargetUser(), target.Addr(secret.Kind()), secret.Kind()))
		plan.add(domain.StepVerified, fmt.Sprintf("authenticate as %q with the new password",
			target.EffectiveTargetUser()))
	} else {
		plan.add(domain.StepApplied, "no backing service to update (generic secret)")
	}

	for _, path := range secret.EnvFiles() {
		plan.add(domain.StepFilesUpdated, fmt.Sprintf("write %s in %s", secret.EnvKey(), path))
	}

	for _, c := range containers {
		plan.add(domain.StepRestarted, fmt.Sprintf("restart %s and wait for it to be healthy", c))
	}
	if len(containers) == 0 {
		plan.add(domain.StepRestarted, "no containers to restart")
	}

	plan.add(domain.StepDone, "record the outcome in the audit trail")

	return plan, nil
}

func (p *RotationPlan) add(step domain.Step, description string) {
	p.Actions = append(p.Actions, PlannedAction{Step: step, Description: description})
}
