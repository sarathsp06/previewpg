package proxy

import (
	"context"
	"fmt"
	"log"

	"github.com/jackc/pgx/v5/pgxpool"
	wire "github.com/jeroenrinzema/psql-wire"
	"strings"

	"github.com/sarathsp06/preview-sql-proxy/internal/config"
	extparser "github.com/xwb1989/sqlparser"
)

type Proxy struct {
	prodDB  *pgxpool.Pool
	freshDB *pgxpool.Pool
	cfg     config.Config
}

func New(prodDB, freshDB *pgxpool.Pool, cfg config.Config) *Proxy {
	p := &Proxy{
		prodDB:  prodDB,
		freshDB: freshDB,
		cfg:     cfg,
	}
	if err := p.setupFDW(context.Background()); err != nil {
		log.Printf("Error setting up FDW: %v. The proxy will continue to work in fallback mode.", err)
	}
	return p
}

func (p *Proxy) setupFDW(ctx context.Context) error {
	log.Println("Setting up Foreign Data Wrapper...")

	// 1. Create Extension
	if _, err := p.freshDB.Exec(ctx, "CREATE EXTENSION IF NOT EXISTS postgres_fdw"); err != nil {
		return fmt.Errorf("failed to create postgres_fdw extension: %w", err)
	}
	log.Println("FDW extension 'postgres_fdw' created or already exists.")

	// 2. Create Server
	dropServerSQL := "DROP SERVER IF EXISTS prod_server CASCADE"
	if _, err := p.freshDB.Exec(ctx, dropServerSQL); err != nil {
		return fmt.Errorf("failed to drop existing server: %w", err)
	}

	createServerSQL := fmt.Sprintf("CREATE SERVER prod_server FOREIGN DATA WRAPPER postgres_fdw OPTIONS (host '%s', port '%d', dbname '%s')",
		p.cfg.ProductionDB.Host, p.cfg.ProductionDB.Port, p.cfg.ProductionDB.DBName)
	if _, err := p.freshDB.Exec(ctx, createServerSQL); err != nil {
		return fmt.Errorf("failed to create foreign server: %w", err)
	}
	log.Println("Foreign server 'prod_server' created.")

	// 3. Create User Mapping
	dropUserMappingSQL := fmt.Sprintf("DROP USER MAPPING IF EXISTS FOR %s SERVER prod_server", "CURRENT_USER")
	if _, err := p.freshDB.Exec(ctx, dropUserMappingSQL); err != nil {
		return fmt.Errorf("failed to drop existing user mapping: %w", err)
	}

	createUserMappingSQL := fmt.Sprintf("CREATE USER MAPPING FOR %s SERVER prod_server OPTIONS (user '%s', password '%s')",
		"CURRENT_USER", p.cfg.ProductionDB.User, p.cfg.ProductionDB.Password)
	if _, err := p.freshDB.Exec(ctx, createUserMappingSQL); err != nil {
		return fmt.Errorf("failed to create user mapping: %w", err)
	}
	log.Println("User mapping for 'prod_server' created.")

	// 4. Introspect schema and create foreign tables
	if err := p.createForeignTables(ctx); err != nil {
		return fmt.Errorf("failed to create foreign tables: %w", err)
	}

	log.Println("FDW setup completed successfully.")
	return nil
}

func (p *Proxy) createForeignTables(ctx context.Context) error {
	tables, err := p.introspectSchema(ctx)
	if err != nil {
		return err
	}
	log.Printf("Found tables in production DB: %v", tables)

	for _, table := range tables {
		// For now, we'll use a simplified, hardcoded schema for the `users` table.
		// A full implementation would dynamically build the column definitions.
		if table == "users" {
			foreignTableName := "prod_users"
			createQuery := fmt.Sprintf(`
				CREATE FOREIGN TABLE %s (
					id INTEGER,
					name TEXT
				) SERVER prod_server OPTIONS (table_name '%s');
			`, foreignTableName, table)

	log.Printf("Executing query on fresh DB: %s", createQuery)
			if _, err := p.freshDB.Exec(ctx, createQuery); err != nil {
				return fmt.Errorf("failed to create foreign table %s: %w", foreignTableName, err)
			}
		}
	}

	return nil
}

func (p *Proxy) introspectSchema(ctx context.Context) ([]string, error) {
	query := `
		SELECT table_name
		FROM information_schema.tables
		WHERE table_schema = 'public' AND table_type = 'BASE TABLE';
	`
	rows, err := p.prodDB.Query(ctx, query)
	if err != nil {
		return nil, fmt.Errorf("querying information_schema.tables failed: %w", err)
	}
	defer rows.Close()

	var tables []string
	for rows.Next() {
		var tableName string
		if err := rows.Scan(&tableName); err != nil {
			return nil, fmt.Errorf("scanning table name failed: %w", err)
		}
		tables = append(tables, tableName)
	}

	if err := rows.Err(); err != nil {
		return nil, fmt.Errorf("error iterating over schema rows: %w", err)
	}

	return tables, nil
}

func (p *Proxy) ListenAndServe() error {
	address := fmt.Sprintf("%s:%d", p.cfg.Server.Host, p.cfg.Server.Port)
	log.Printf("Starting proxy on %s", address)
	return wire.ListenAndServe(address, p.queryHandler)
}

func (p *Proxy) rewriteSelectQuery(query string) (string, error) {
	stmt, err := extparser.Parse(query)
	if err != nil {
		return "", fmt.Errorf("failed to parse query: %w", err)
	}

	selectStmt, ok := stmt.(*extparser.Select)
	if !ok {
		// Not a select statement, return as is
		return query, nil
	}

	// Basic validation: only handle single table selects for now
	if len(selectStmt.From) != 1 {
		return query, nil
	}
	tableNameNode, ok := selectStmt.From[0].(*extparser.AliasedTableExpr)
	if !ok {
		return query, nil
	}
	tableName := extparser.String(tableNameNode.Expr)

	// We only rewrite queries for the 'users' table for now
	if tableName != "users" {
		return query, nil
	}

	// Get column names
	var columns []string
	for _, sel := range selectStmt.SelectExprs {
		col, ok := sel.(*extparser.AliasedExpr)
		if !ok {
			// Handle '*' case
			if _, ok := sel.(*extparser.StarExpr); ok {
				// This is a simplified case. A real implementation would look up table columns.
				columns = []string{"id", "name"}
				break
			}
			continue
		}
		columns = append(columns, extparser.String(col.Expr))
	}
	columnList := strings.Join(columns, ", ")

	// Build the rewritten query
	rewrittenQuery := fmt.Sprintf(`
		WITH fresh_ids AS (
			SELECT id FROM users
		)
		SELECT %s FROM users
		UNION ALL
		SELECT %s FROM prod_users WHERE id NOT IN (SELECT id FROM fresh_ids)
	`, columnList, columnList)

	if selectStmt.OrderBy != nil {
		rewrittenQuery += " " + extparser.String(selectStmt.OrderBy)
	}

	rewrittenQuery += ";"

	return rewrittenQuery, nil
}

func (p *Proxy) queryHandler(ctx context.Context, query string) (wire.PreparedStatements, error) {
	log.Printf("Received query: %s", query)

	stmt, err := extparser.Parse(query)
	if err != nil {
		log.Printf("Failed to parse query, defaulting to fresh DB: %v", err)
		return p.execute(p.freshDB, query), nil
	}

	switch stmt.(type) {
	case *extparser.Select:
		log.Printf("Routing READ to be rewritten")
		rewrittenQuery, err := p.rewriteSelectQuery(query)
		if err != nil {
			log.Printf("Failed to rewrite query, falling back to fresh DB: %v", err)
			return p.execute(p.freshDB, query), nil
		}
		log.Printf("Rewritten query: %s", rewrittenQuery)
		return p.execute(p.freshDB, rewrittenQuery), nil
	case *extparser.Insert, *extparser.Update, *extparser.Delete, *extparser.DDL, *extparser.Begin, *extparser.Commit, *extparser.Rollback:
		log.Printf("Routing WRITE/DDL/TRANSACTION to fresh DB")
		return p.execute(p.freshDB, query), nil
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
				log.Printf("Error getting row values: %v", err)
				return err
			}
			log.Printf("Row values: %v", values)
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