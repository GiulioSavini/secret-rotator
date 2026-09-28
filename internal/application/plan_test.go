package application

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/giulio/secret-rotator/internal/domain"
)

func TestPlanDescribesTheWholePipelineWithoutTouchingAnything(t *testing.T) {
	env := newFakeEnvStore()
	env.put(".env", "DB_PASSWORD", "old-value")
	rotator := &fakeRotator{kind: domain.KindPostgres}
	fleet := &fakeFleet{}

	plan, err := NewPlanRotation(fakeRegistry{rotator: rotator}, env, fleet).
		Build(context.Background(), secretFor(t, nil))
	require.NoError(t, err)

	joined := strings.Join(descriptions(plan), "\n")
	assert.Contains(t, joined, "read the current DB_PASSWORD")
	assert.Contains(t, joined, "generate a new 32-byte value")
	assert.Contains(t, joined, "set the password of \"postgres\" on db:5432")
	assert.Contains(t, joined, "write DB_PASSWORD in .env")
	assert.Contains(t, joined, "restart db")
	assert.Contains(t, joined, "restart app")

	// Nothing may have happened.
	assert.Empty(t, rotator.applied)
	assert.Empty(t, env.writes)
	assert.Empty(t, fleet.restarts)
	assert.Equal(t, "old-value", env.valueOf(".env", "DB_PASSWORD"))
}

func TestPlanReportsAGenericSecretHasNoService(t *testing.T) {
	env := newFakeEnvStore()
	env.put(".env", "APP_SECRET_KEY", "old-value")

	secret := secretFor(t, func(spec *domain.SecretSpec) {
		spec.Kind = "generic"
		spec.EnvKey = "APP_SECRET_KEY"
		spec.Target = domain.Target{}
		spec.Containers = nil
	})

	plan, err := NewPlanRotation(fakeRegistry{rotator: &fakeRotator{kind: domain.KindGeneric}}, env, &fakeFleet{}).
		Build(context.Background(), secret)
	require.NoError(t, err)

	joined := strings.Join(descriptions(plan), "\n")
	assert.Contains(t, joined, "no backing service")
	assert.Contains(t, joined, "no containers to restart")
}

func TestPlanFailsOnAMissingKey(t *testing.T) {
	env := newFakeEnvStore()
	env.put(".env", "SOMETHING_ELSE", "value")

	_, err := NewPlanRotation(fakeRegistry{rotator: &fakeRotator{kind: domain.KindPostgres}}, env, &fakeFleet{}).
		Build(context.Background(), secretFor(t, nil))

	require.Error(t, err)
	assert.ErrorIs(t, err, domain.ErrSecretKeyMissing)
}

func TestPlanFailsOnAnUnresolvableContainer(t *testing.T) {
	env := newFakeEnvStore()
	env.put(".env", "DB_PASSWORD", "old-value")
	fleet := &fakeFleet{resolveErr: errors.New(`container "app" not found`)}

	_, err := NewPlanRotation(fakeRegistry{rotator: &fakeRotator{kind: domain.KindPostgres}}, env, fleet).
		Build(context.Background(), secretFor(t, nil))

	require.Error(t, err)
	assert.Contains(t, err.Error(), `container "app" not found`)
}

func TestPlanShowsResolvedContainerNames(t *testing.T) {
	env := newFakeEnvStore()
	env.put(".env", "DB_PASSWORD", "old-value")
	fleet := &fakeFleet{resolveTo: []domain.ContainerRef{"proj-db-1", "proj-app-1"}}

	plan, err := NewPlanRotation(fakeRegistry{rotator: &fakeRotator{kind: domain.KindPostgres}}, env, fleet).
		Build(context.Background(), secretFor(t, nil))
	require.NoError(t, err)

	joined := strings.Join(descriptions(plan), "\n")
	assert.Contains(t, joined, "proj-db-1", "the plan must name the containers that would really restart")
	assert.Contains(t, joined, "proj-app-1")
}

func descriptions(plan *RotationPlan) []string {
	out := make([]string, 0, len(plan.Actions))
	for _, a := range plan.Actions {
		out = append(out, a.Description)
	}
	return out
}
