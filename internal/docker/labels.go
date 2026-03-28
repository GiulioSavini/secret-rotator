package docker

import (
	"context"
	"fmt"
	"strings"

	"github.com/robfig/cron/v3"
)

const (
	labelPrefix   = "com.secret-rotator."
	labelSuffix   = ".schedule"
	globalLabel   = "com.secret-rotator.schedule"
)

// ScheduleLabel represents a cron schedule discovered from a Docker container label.
type ScheduleLabel struct {
	ContainerName string // Docker container name
	SecretName    string // Empty string for global schedules
	CronExpr      string // Cron expression (standard 5-field or predefined like @daily)
}

// ReadScheduleLabels discovers cron schedules from Docker container labels.
// It looks for labels matching:
//   - com.secret-rotator.schedule (global schedule for all secrets)
//   - com.secret-rotator.{name}.schedule (per-secret schedule)
func ReadScheduleLabels(ctx context.Context, mgr Manager) ([]ScheduleLabel, error) {
	containers, err := mgr.ListContainers(ctx, ContainerFilter{})
	if err != nil {
		return nil, fmt.Errorf("listing containers: %w", err)
	}

	parser := cron.NewParser(cron.Minute | cron.Hour | cron.Dom | cron.Month | cron.Dow | cron.Descriptor)

	var labels []ScheduleLabel
	for _, c := range containers {
		for key, value := range c.Labels {
			if key == globalLabel {
				if _, err := parser.Parse(value); err != nil {
					return nil, fmt.Errorf("invalid cron expression %q on container %s: %w", value, c.Name, err)
				}
				labels = append(labels, ScheduleLabel{
					ContainerName: c.Name,
					SecretName:    "",
					CronExpr:      value,
				})
			} else if strings.HasPrefix(key, labelPrefix) && strings.HasSuffix(key, labelSuffix) {
				// Extract secret name: strip prefix and suffix
				secretName := strings.TrimPrefix(key, labelPrefix)
				secretName = strings.TrimSuffix(secretName, labelSuffix)
				if secretName == "" {
					continue // Already handled by globalLabel
				}
				if _, err := parser.Parse(value); err != nil {
					return nil, fmt.Errorf("invalid cron expression %q on container %s for secret %s: %w",
						value, c.Name, secretName, err)
				}
				labels = append(labels, ScheduleLabel{
					ContainerName: c.Name,
					SecretName:    secretName,
					CronExpr:      value,
				})
			}
		}
	}

	return labels, nil
}
