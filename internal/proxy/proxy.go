package proxy

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	wire "github.com/jeroenrinzema/psql-wire"
	"github.com/sarathsp06/preview-sql-proxy/internal/config"
	"github.com/sarathsp06/preview-sql-proxy/internal/sqlparser"
)

type Proxy struct {
	prodDB  *pgxpool.Pool
	freshDB *pgxpool.Pool
	cfg     config.Config
}

func New(prodDB, freshDB *pgxpool.Pool, cfg config.Config) *Proxy {
	return &Proxy{
		prodDB:  prodDB,
		freshDB: freshDB,
		cfg:     cfg,
	}
}

func (p *Proxy) ListenAndServe() error {
	address := fmt.Sprintf("%s:%d", p.cfg.Server.Host, p.cfg.Server.Port)
	log.Printf("Starting proxy on %s", address)
	return wire.ListenAndServe(address, p.queryHandler)
}

func (p *Proxy) queryHandler(ctx context.Context, query string) (wire.PreparedStatements, error) {
	log.Printf("Received query: %s", query)

	stmtType, err := sqlparser.GetStmtType(query)
	if err != nil {
		log.Printf("Failed to parse query, defaulting to fresh DB: %v", err)
		return p.execute(p.freshDB, query), nil
	}

	switch stmtType {
	case sqlparser.Write, sqlparser.DDL, sqlparser.Transaction:
		log.Printf("Routing WRITE/DDL/TRANSACTION to fresh DB")
		return p.execute(p.freshDB, query), nil
	case sqlparser.Read:
		log.Printf("Routing READ to fresh DB")
		// First, try the fresh database
		rows, err := p.freshDB.Query(ctx, query)
		// If there's no error and we get rows, we can return them.
		// A bit tricky to check if rows were returned without consuming them.
		// For now, we'll check the error. If the table/column doesn't exist, it will error.
		if err == nil {
			rows.Close() // Close the rows, we will re-query in the executor
			log.Printf("Query successful on fresh DB")
			return p.execute(p.freshDB, query), nil
		}

		log.Printf("Query failed on fresh DB (%v), falling back to prod DB", err)
		return p.execute(p.prodDB, query), nil
	default:
		log.Printf("Unknown statement type, defaulting to fresh DB")
		return p.execute(p.freshDB, query), nil
	}
}

func (p *Proxy) execute(db *pgxpool.Pool, query string) wire.PreparedStatements {
	stmt := wire.NewStatement(func(ctx context.Context, writer wire.DataWriter, parameters []wire.Parameter) error {
		rows, err := db.Query(ctx, query)
		if err != nil {
			log.Printf("Error executing query on backend: %v", err)
			return writer.Complete("ERROR")
		}
		defer rows.Close()

		for rows.Next() {
			values, err := rows.Values()
			if err != nil {
				return err
			}
			row := make([]any, len(values))
			for i, v := range values {
				row[i] = fmt.Sprintf("%v", v)
			}
			writer.Row(row)
		}

		if err := rows.Err(); err != nil {
			return err
		}

		return writer.Complete(rows.CommandTag().String())
	})
	return wire.Prepared(stmt)
}