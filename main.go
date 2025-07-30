//go:build !lambda

package main

import (
	"context"
	"fmt"
	"github.com/gw2auth/gw2auth.com-api/telemetry"
	"github.com/labstack/echo/v4"
	"io"
	"os"
	"os/signal"
	"path/filepath"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt)
	defer stop()

	err := WithTelemetry(
		ctx,
		"GW2AuthAPILambda",
		func(ctx context.Context, t *telemetry.Telemetry) error {
			return WithEchoServer(ctx, func(ctx context.Context, app *echo.Echo) error {
				go func() {
					<-ctx.Done()
					if err := app.Shutdown(context.Background()); err != nil {
						fmt.Printf("error shutting down echo server: %v\n", err)
					} else {
						fmt.Println("shutdown complete")
					}
				}()

				return app.Start(":8090")
			}, WithFlusher(t))
		},
		telemetry.WithResource(telemetry.NewLocalResource),
		telemetry.WithTracerProvider(telemetry.NewLocalTracerProvider),
		telemetry.WithLoggerProvider(telemetry.NewLocalLoggerProvider),
	)

	if err != nil {
		panic(err)
	}
}

func loadSecrets(ctx context.Context) (Secrets, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return Secrets{}, err
	}

	pub1, err := loadFile(filepath.Join(home, ".gw2auth", "session_id_rsa_1.pub"))
	if err != nil {
		return Secrets{}, err
	}

	pub2, err := loadFile(filepath.Join(home, ".gw2auth", "session_id_rsa_2.pub"))
	if err != nil {
		return Secrets{}, err
	}

	priv, err := loadFile(filepath.Join(home, ".gw2auth", "session_id_rsa_2"))
	if err != nil {
		return Secrets{}, err
	}

	return Secrets{
		DatabaseURL:           "postgres://gw2auth_app:@localhost:26257/defaultdb",
		SessionRSAPublicKid1:  "0412f229-a208-45d1-8c00-93f1ff92d24b",
		SessionRSAPublicKid2:  "dbb703c6-87ec-4329-844e-01d03e373dac",
		SessionRSAPrivateKid2: "dbb703c6-87ec-4329-844e-01d03e373dac",
		SessionRSAPublicPEM1:  pub1,
		SessionRSAPublicPEM2:  pub2,
		SessionRSAPrivatePEM2: priv,
	}, nil
}

func loadFile(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()

	b, err := io.ReadAll(f)
	if err != nil {
		return "", err
	}

	return string(b), nil
}
