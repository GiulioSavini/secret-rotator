package docker

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestReadScheduleLabelsGlobalSchedule(t *testing.T) {
	mock := &MockManager{
		ListFunc: func(ctx context.Context, filter ContainerFilter) ([]Container, error) {
			return []Container{
				{
					ID:   "c1",
					Name: "myapp",
					Labels: map[string]string{
						"com.secret-rotator.schedule": "0 2 * * *",
					},
				},
			}, nil
		},
	}

	labels, err := ReadScheduleLabels(context.Background(), mock)
	require.NoError(t, err)
	require.Len(t, labels, 1)
	assert.Equal(t, "myapp", labels[0].ContainerName)
	assert.Equal(t, "", labels[0].SecretName)
	assert.Equal(t, "0 2 * * *", labels[0].CronExpr)
}

func TestReadScheduleLabelsPerSecretSchedule(t *testing.T) {
	mock := &MockManager{
		ListFunc: func(ctx context.Context, filter ContainerFilter) ([]Container, error) {
			return []Container{
				{
					ID:   "c1",
					Name: "mydb",
					Labels: map[string]string{
						"com.secret-rotator.db_pass.schedule": "@daily",
					},
				},
			}, nil
		},
	}

	labels, err := ReadScheduleLabels(context.Background(), mock)
	require.NoError(t, err)
	require.Len(t, labels, 1)
	assert.Equal(t, "mydb", labels[0].ContainerName)
	assert.Equal(t, "db_pass", labels[0].SecretName)
	assert.Equal(t, "@daily", labels[0].CronExpr)
}

func TestReadScheduleLabelsIgnoresNonScheduleLabels(t *testing.T) {
	mock := &MockManager{
		ListFunc: func(ctx context.Context, filter ContainerFilter) ([]Container, error) {
			return []Container{
				{
					ID:   "c1",
					Name: "myapp",
					Labels: map[string]string{
						"com.docker.compose.service": "web",
						"maintainer":                 "someone",
					},
				},
			}, nil
		},
	}

	labels, err := ReadScheduleLabels(context.Background(), mock)
	require.NoError(t, err)
	assert.Empty(t, labels)
}

func TestScheduleLabelFields(t *testing.T) {
	mock := &MockManager{
		ListFunc: func(ctx context.Context, filter ContainerFilter) ([]Container, error) {
			return []Container{
				{
					ID:   "c1",
					Name: "webapp",
					Labels: map[string]string{
						"com.secret-rotator.schedule":             "0 3 * * 0",
						"com.secret-rotator.api_key.schedule":     "@weekly",
						"com.secret-rotator.db_password.schedule": "0 0 1 * *",
					},
				},
			}, nil
		},
	}

	labels, err := ReadScheduleLabels(context.Background(), mock)
	require.NoError(t, err)
	require.Len(t, labels, 3)

	// Build a map for easier assertion
	byName := make(map[string]ScheduleLabel)
	for _, l := range labels {
		key := l.SecretName
		if key == "" {
			key = "_global_"
		}
		byName[key] = l
	}

	assert.Equal(t, "webapp", byName["_global_"].ContainerName)
	assert.Equal(t, "0 3 * * 0", byName["_global_"].CronExpr)

	assert.Equal(t, "webapp", byName["api_key"].ContainerName)
	assert.Equal(t, "@weekly", byName["api_key"].CronExpr)

	assert.Equal(t, "webapp", byName["db_password"].ContainerName)
	assert.Equal(t, "0 0 1 * *", byName["db_password"].CronExpr)
}

func TestReadScheduleLabelsEmptyContainerList(t *testing.T) {
	mock := &MockManager{
		ListFunc: func(ctx context.Context, filter ContainerFilter) ([]Container, error) {
			return []Container{}, nil
		},
	}

	labels, err := ReadScheduleLabels(context.Background(), mock)
	require.NoError(t, err)
	assert.Empty(t, labels)
}

func TestReadScheduleLabelsInvalidCron(t *testing.T) {
	mock := &MockManager{
		ListFunc: func(ctx context.Context, filter ContainerFilter) ([]Container, error) {
			return []Container{
				{
					ID:   "c1",
					Name: "myapp",
					Labels: map[string]string{
						"com.secret-rotator.schedule": "not a cron expression",
					},
				},
			}, nil
		},
	}

	_, err := ReadScheduleLabels(context.Background(), mock)
	require.Error(t, err)
	assert.Contains(t, err.Error(), "invalid cron")
}
