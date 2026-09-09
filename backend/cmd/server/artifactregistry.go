package main

import (
	"context"
	"fmt"

	"github.com/OpenNSW/core/artifact"
	"github.com/OpenNSW/core/artifact/loaders"
)

// newArtifactRegistry builds the shared artifact registry from cfg's artifact
// root.
func newArtifactRegistry(ctx context.Context, cfg Config) (*artifact.Registry, error) {
	loader, err := loaders.New(ctx, cfg.ArtifactLoader)
	if err != nil {
		return nil, fmt.Errorf("failed to initialize artifact loader: %w", err)
	}

	registry := artifact.NewRegistry(loader)

	manifestCfg, err := artifact.LoadManifest(ctx, loader)
	if err != nil {
		return nil, fmt.Errorf("failed to load artifact manifest: %w", err)
	}
	if err := artifact.RegisterFromConfig(registry, manifestCfg); err != nil {
		return nil, fmt.Errorf("failed to register artifacts from manifest: %w", err)
	}

	return registry, nil
}
