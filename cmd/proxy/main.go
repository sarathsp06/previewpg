package main

import (
	"log"

	"github.com/sarathsp06/preview-sql-proxy/internal/config"
	"github.com/sarathsp06/preview-sql-proxy/internal/database"
	"github.com/sarathsp06/preview-sql-proxy/internal/proxy"
)

func main() {
	cfg, err := config.LoadConfig()
	if err != nil {
		log.Fatalf("Failed to load config: %v", err)
	}

	prodDB, err := database.Connect(cfg.ProductionDB)
	if err != nil {
		log.Fatalf("Failed to connect to production DB: %v", err)
	}
	defer prodDB.Close()

	freshDB, err := database.Connect(cfg.FreshDB)
	if err != nil {
		log.Fatalf("Failed to connect to fresh DB: %v", err)
	}
	defer freshDB.Close()

	p := proxy.New(prodDB, freshDB, cfg)

	err = p.ListenAndServe()
	if err != nil {
		log.Fatal(err)
	}
}
