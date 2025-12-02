package containers

import (
	"context"

	"github.com/docker/docker/api/types/container"
)

type Host interface {
	ContainerList(ctx context.Context, clo container.ListOptions) ([]container.Summary, error)

	ContainerStart(ctx context.Context, id string, opt container.StartOptions) error
	ContainerStop(ctx context.Context, id string, opt container.StopOptions) error

	ContainerStatsOneShot(ctx context.Context, id string) (container.StatsResponseReader, error)

	Close() error
}
