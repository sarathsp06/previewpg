package e2e

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/ory/dockertest/v3"
	"github.com/ory/dockertest/v3/docker"
	"github.com/sarathsp06/preview-sql-proxy/internal/config"
	"github.com/sarathsp06/preview-sql-proxy/internal/database"
	"github.com/sarathsp06/preview-sql-proxy/internal/proxy"
)

var (
	prodDB        *pgxpool.Pool
	freshDB       *pgxpool.Pool
	prodDBConfig  config.DBConfig
	freshDBConfig config.DBConfig
	prodResource  *dockertest.Resource
)

func TestMain(m *testing.M) {
	pool, err := dockertest.NewPool("")
	if err != nil {
		log.Fatalf("Could not connect to docker: %s", err)
	}

	prodResource, err = pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_PASSWORD=password",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=production",
			"listen_addresses = '*'",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		log.Fatalf("Could not start production resource: %s", err)
	}

	freshResource, err := pool.RunWithOptions(&dockertest.RunOptions{
		Repository: "postgres",
		Tag:        "16-alpine",
		Env: []string{
			"POSTGRES_PASSWORD=password",
			"POSTGRES_USER=postgres",
			"POSTGRES_DB=fresh",
			"listen_addresses = '*'",
		},
	}, func(config *docker.HostConfig) {
		config.AutoRemove = true
		config.RestartPolicy = docker.RestartPolicy{Name: "no"}
	})
	if err != nil {
		log.Fatalf("Could not start fresh resource: %s", err)
	}

	prodHostAndPort := prodResource.GetHostPort("5432/tcp")
	freshHostAndPort := freshResource.GetHostPort("5432/tcp")
	log.Println("Production DB running on:", prodHostAndPort)
	log.Println("Fresh DB running on:", freshHostAndPort)

	if err := pool.Retry(func() error {
		var err error
		prodHost, prodPortStr, _ := strings.Cut(prodHostAndPort, ":")
		prodPort, _ := strconv.Atoi(prodPortStr)
		prodDBConfig = config.DBConfig{
			Host:     prodHost,
			Port:     prodPort,
			User:     "postgres",
			Password: "password",
			DBName:   "production",
			SSLMode:  "disable",
		}
		prodDB, err = database.Connect(prodDBConfig)
		if err != nil {
			return err
		}

		freshHost, freshPortStr, _ := strings.Cut(freshHostAndPort, ":")
		freshPort, _ := strconv.Atoi(freshPortStr)
		freshDBConfig = config.DBConfig{
			Host:     freshHost,
			Port:     freshPort,
			User:     "postgres",
			Password: "password",
			DBName:   "fresh",
			SSLMode:  "disable",
		}
		freshDB, err = database.Connect(freshDBConfig)
		if err != nil {
			return err
		}

		return prodDB.Ping(context.Background())
	}); err != nil {
		log.Fatalf("Could not connect to databases: %s", err)
	}

	code := m.Run()

	if err := pool.Purge(prodResource); err != nil {
		log.Fatalf("Could not purge production resource: %s", err)
	}
	if err := pool.Purge(freshResource); err != nil {
		log.Fatalf("Could not purge fresh resource: %s", err)
	}

	os.Exit(code)
}

func startProxy(port int, prodContainer *dockertest.Resource) (string, func(), error) {
	server := &http.Server{Addr: fmt.Sprintf("localhost:%d", port)}

	// We need to update the production DB host to use the container's IP address
	// so that the fresh DB container can connect to it.
	prodDBConfigForProxy := prodDBConfig
	prodDBConfigForProxy.Host = prodContainer.Container.NetworkSettings.IPAddress

	cfg := config.Config{
		Server: config.ServerConfig{
			Host: "localhost",
			Port: port,
		},
		ProductionDB: prodDBConfigForProxy,
		FreshDB:      freshDBConfig,
	}
	p := proxy.New(prodDB, freshDB, cfg)

	go func() {
		if err := p.ListenAndServe(); err != http.ErrServerClosed {
			log.Printf("proxy listen and serve failed: %s", err)
		}
	}()

	stop := func() {
		// Shutdown the server
		if err := server.Shutdown(context.Background()); err != nil {
			log.Printf("proxy shutdown failed: %s", err)
		}
	}

	// Wait for the proxy to be ready
	time.Sleep(2 * time.Second)

	connStr := fmt.Sprintf("postgres://postgres:password@localhost:%d/fresh?sslmode=disable", port)
	return connStr, stop, nil
}
