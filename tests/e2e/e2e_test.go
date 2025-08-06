package e2e

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
	"github.com/testcontainers/testcontainers-go"
	"github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
)

func TestContainersRunning(t *testing.T) {
	ctx := context.Background()

	// Define a more robust wait strategy to ensure the database is ready.
	waitStrategy := wait.ForLog("database system is ready to accept connections").
		WithOccurrence(2).
		WithStartupTimeout(5 * time.Minute)

	// Create production container with the wait strategy
	prodContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("docker.io/postgres:16-alpine"),
		testcontainers.WithWaitStrategy(waitStrategy),
	)
	assert.NoError(t, err)
	defer func() {
		if err := prodContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate prod container: %s", err)
		}
	}()

	// Create fresh container with the wait strategy
	freshContainer, err := postgres.RunContainer(ctx,
		testcontainers.WithImage("docker.io/postgres:16-alpine"),
		testcontainers.WithWaitStrategy(waitStrategy),
	)
	assert.NoError(t, err)
	defer func() {
		if err := freshContainer.Terminate(ctx); err != nil {
			t.Fatalf("failed to terminate fresh container: %s", err)
		}
	}()

	// Get connection strings
	prodConnStr, err := prodContainer.ConnectionString(ctx, "sslmode=disable")
	assert.NoError(t, err)
	freshConnStr, err := freshContainer.ConnectionString(ctx, "sslmode=disable")
	assert.NoError(t, err)

	// Connect to production DB
	t.Logf("Production connection string: %s", prodConnStr)
	prodConn, err := pgx.Connect(ctx, prodConnStr)
	assert.NoError(t, err)
	defer prodConn.Close(ctx)

	// Connect to fresh DB
	t.Logf("Fresh connection string: %s", freshConnStr)
	freshConn, err := pgx.Connect(ctx, freshConnStr)
	assert.NoError(t, err)
	defer freshConn.Close(ctx)

	// Ping databases
	err = prodConn.Ping(ctx)
	assert.NoError(t, err)

	err = freshConn.Ping(ctx)
	assert.NoError(t, err)
}