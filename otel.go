package main

import (
	"context"
	"errors"
	"github.com/gw2auth/gw2auth.com-api/telemetry"
)

func WithTelemetry(ctx context.Context, serviceName string, fn func(ctx context.Context, t *telemetry.Telemetry) error, options ...telemetry.Option) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()

	t, err := telemetry.NewTelemetry(ctx, serviceName, options...)
	if err != nil {
		return err
	}

	err = fn(ctx, t)
	if e := t.Shutdown(ctx); e != nil {
		err = errors.Join(err, e)
	}

	return err
}
