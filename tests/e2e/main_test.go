package e2e

import (
	"context"
	"testing"

	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
)

func TestMain(m *testing.M) {
	// setup
	ctx := context.Background()

	prodContainer, err := postgres.RunContainer(ctx, testcontainers.WithImage("docker.io/postgres:16-alpine"))
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := prodContainer.Terminate(ctx); err != nil {
			panic(err)
		}
	}()

	freshContainer, err := postgres.RunContainer(ctx, testcontainers.WithImage("docker.io/postgres:16-alpine"))
	if err != nil {
		panic(err)
	}
	defer func() {
		if err := freshContainer.Terminate(ctx); err != nil {
			panic(err)
		}
	}()

	// run tests
	m.Run()
}
