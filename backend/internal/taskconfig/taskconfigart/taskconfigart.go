// Package taskconfigart adapts taskconfig.TaskConfig to the core/artifact
// registry. It is the only layer that imports both taskconfig and artifact,
// keeping taskconfig itself free of registry concerns.
package taskconfigart

import (
	"context"
	"encoding/json"
	"fmt"

	"github.com/OpenNSW/core/artifact"

	"github.com/OpenNSW/nsw-agency/backend/internal/taskconfig"
)

// Kind is the artifact kind owned by this adapter.
const Kind artifact.Kind = "task_config"

// loadable wraps taskconfig.TaskConfig so it satisfies artifact.Artifact and
// artifact.Parser without polluting the domain type.
type loadable struct {
	taskconfig.TaskConfig
}

// Kind reports a constant kind from a value receiver, as the registry requires.
func (loadable) Kind() artifact.Kind { return Kind }

// Parse populates and validates the task config from raw bytes.
func (l *loadable) Parse(raw []byte) error {
	var c taskconfig.TaskConfig
	if err := json.Unmarshal(raw, &c); err != nil {
		return fmt.Errorf("decode task config: %w", err)
	}
	if err := c.Validate(); err != nil {
		return fmt.Errorf("invalid task config: %w", err)
	}
	l.TaskConfig = c
	return nil
}

// Load fetches and parses the newest version of the task config with the given id.
func Load(ctx context.Context, reg *artifact.Registry, id string) (taskconfig.TaskConfig, error) {
	w, err := artifact.Latest[loadable](ctx, reg, id)
	if err != nil {
		return taskconfig.TaskConfig{}, err
	}
	return w.TaskConfig, nil
}
