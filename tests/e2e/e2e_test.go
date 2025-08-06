package e2e

import (
	"context"
	"testing"

	"github.com/jackc/pgx/v5"
	"github.com/stretchr/testify/assert"
)

func TestFDWSetup(t *testing.T) {
	ctx := context.Background()

	// Create tables in both databases
	_, err := prodDB.Exec(ctx, `CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT)`)
	assert.NoError(t, err)
	_, err = freshDB.Exec(ctx, `CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT)`)
	assert.NoError(t, err)
	defer func() {
		_, err := prodDB.Exec(ctx, `DROP TABLE users`)
		assert.NoError(t, err)
		_, err = freshDB.Exec(ctx, `DROP TABLE users`)
		assert.NoError(t, err)
	}()

	// Start the proxy
	proxyConnStr, stop, err := startProxy(15432, prodResource)
	assert.NoError(t, err)
	defer stop()

	// Connect to proxy
	proxyConn, err := pgx.Connect(ctx, proxyConnStr)
	assert.NoError(t, err)
	defer proxyConn.Close(ctx)

	// Ping proxy
	err = proxyConn.Ping(ctx)
	assert.NoError(t, err)

	// Verify that the foreign table was created in the fresh database
	var tableName string
	err = freshDB.QueryRow(ctx, "SELECT ftrelid::regclass::text FROM pg_foreign_table").Scan(&tableName)
	assert.NoError(t, err)
	assert.Equal(t, "prod_users", tableName)
}

func TestQueryFederation(t *testing.T) {
	ctx := context.Background()

	// Create tables in both databases
	_, err := prodDB.Exec(ctx, `CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT)`)
	assert.NoError(t, err)
	_, err = freshDB.Exec(ctx, `CREATE TABLE users (id SERIAL PRIMARY KEY, name TEXT)`)
	assert.NoError(t, err)
	defer func() {
		_, err := prodDB.Exec(ctx, `DROP TABLE users`)
		assert.NoError(t, err)
		_, err = freshDB.Exec(ctx, `DROP TABLE users`)
		assert.NoError(t, err)
	}()

	// 1. Seed Production DB
	_, err = prodDB.Exec(ctx, "INSERT INTO users (id, name) VALUES (1, 'Prod User 1'), (2, 'Prod User 2')")
	assert.NoError(t, err)

	// 2. Start Proxy
	proxyConnStr, stop, err := startProxy(15433, prodResource)
	assert.NoError(t, err)
	defer stop()

	// 3. Connect to Proxy
	proxyConn, err := pgx.Connect(ctx, proxyConnStr)
	assert.NoError(t, err)
	defer proxyConn.Close(ctx)

	// 4. Query through Proxy
	rows, err := proxyConn.Query(ctx, "SELECT id, name FROM users ORDER BY id")
	assert.NoError(t, err)
	defer rows.Close()

	// 5. Verify Results
	var ids []int
	var names []string
	for rows.Next() {
		var id int
		var name string
		err := rows.Scan(&id, &name)
		assert.NoError(t, err)
		ids = append(ids, id)
		names = append(names, name)
	}

	assert.Equal(t, []int{1, 2}, ids)
	assert.Equal(t, []string{"Prod User 1", "Prod User 2"}, names)
}