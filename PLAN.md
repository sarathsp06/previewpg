# Project Plan: The Schema-Aware SQL Federation Proxy

## 1. Project Vision & Goal

To create a transparent, intelligent PostgreSQL proxy that virtualizes a preview/QA environment. The proxy will automatically federate data between a "production" and a "fresh" database, presenting a unified, consistent view to the client. It will achieve this by programmatically managing PostgreSQL Foreign Data Wrappers (`postgres_fdw`), offloading the complex work of data merging and filtering to the database itself.

The end-user will connect to the proxy and feel as though they are interacting with a single, state-of-the-art database that combines the stability of production data with the flexibility of a development schema.

## 2. Core Architecture

The proxy will be a single Go application with several key internal components:

*   **Proxy Core (`psql-wire`):** The front door. Listens for client connections, accepts raw SQL queries, and streams results back.
*   **Configuration Manager:** Loads connection strings and server settings from a `config.yaml` file and environment variables.
*   **Connection Manager (`pgxpool`):** Manages robust connection pools to the backend "production" and "fresh" databases.
*   **Schema Manager (The Brain):**
    *   On startup, introspects the schemas of both databases to build an in-memory model.
    *   Identifies which tables exist in production.
    *   Tracks schema changes made to the fresh database in real-time.
    *   Provides the "virtual" or "target" schema for any given query.
*   **FDW Manager (The Automation Engine):**
    *   On startup, uses the `Schema Manager`'s information to configure the `Fresh DB`.
    *   It will automatically run `CREATE EXTENSION`, `CREATE SERVER`, `CREATE USER MAPPING`, and `CREATE FOREIGN TABLE` for every table found on the production database.
    *   It will also be responsible for updating or recreating foreign tables if the production schema were to change.
*   **Query Rewriter (The Logic):**
    *   The most critical component. It intercepts incoming queries and rewrites them before execution.
    *   It uses the `Schema Manager` to understand the query's context and transforms it into a single, efficient query that can be run on the `Fresh DB`.

## 3. Detailed Workflow & Logic

This architecture changes the data flow significantly from the simple fallback model.

### A. On Proxy Startup (Automated Setup)

1.  **Load & Connect:** The proxy starts, loads its configuration, and connects to both the production and fresh databases.
2.  **Introspect Schemas:** The `Schema Manager` queries `information_schema` on both databases to learn about all existing tables and columns.
3.  **Configure FDW:** The `FDW Manager` executes the following commands on the `Fresh DB`:
    *   `CREATE EXTENSION IF NOT EXISTS postgres_fdw;`
    *   `CREATE SERVER prod_server FOREIGN DATA WRAPPER postgres_fdw OPTIONS (...)` using the production connection details.
    *   `CREATE USER MAPPING FOR CURRENT_USER SERVER prod_server OPTIONS (...)`.
4.  **Create Foreign Tables:** For every table the `Schema Manager` found in the production database, the `FDW Manager` generates and executes a corresponding `CREATE FOREIGN TABLE prod_users (...) SERVER prod_server ...` command on the `Fresh DB`.
5.  **Ready:** The proxy is now fully initialized and begins listening for client connections.

### B. Handling a `SELECT` Query

1.  **Receive:** The `Proxy Core` receives a query, e.g., `SELECT id, name, last_name FROM users WHERE active = true;`.
2.  **Rewrite:** The query is passed to the `Query Rewriter`.
    *   The rewriter analyzes the query and identifies that it targets the `users` table.
    *   It consults the `Schema Manager` to get the "virtual" schema for `users` (which includes `last_name`) and the name of the corresponding foreign table (`prod_users`).
    *   It constructs a new, single, powerful query designed to be run on the `Fresh DB`:
        ```sql
        -- This is the query the proxy sends to the Fresh DB
        WITH fresh_ids AS (
          SELECT id FROM users
        )
        -- Get all rows from the local (fresh) users table
        SELECT id, name, email, last_name
        FROM users
        WHERE active = true
        UNION ALL
        -- Get all rows from the foreign (production) users table...
        SELECT id, name, email, NULL AS last_name -- ...adding NULL for the new column
        FROM prod_users
        WHERE active = true
          -- ...but only if they haven't been modified or created in the fresh table.
          AND id NOT IN (SELECT id FROM fresh_ids);
        ```
3.  **Execute:** This single, rewritten query is sent **only to the Fresh DB**.
4.  **Federate:** The `Fresh DB`, using the `postgres_fdw` extension, handles all the work of querying its own table, querying the remote production table, and merging the results.
5.  **Respond:** The final, unified result set is streamed from the `Fresh DB` through the proxy back to the client.

### C. Handling a `DDL` (e.g., `ALTER TABLE`) Query

1.  **Receive:** The proxy receives `ALTER TABLE users ADD COLUMN phone VARCHAR;`.
2.  **Classify:** The `Query Rewriter` identifies this as a DDL statement.
3.  **Execute on Fresh DB:** The query is sent directly to the `Fresh DB` for execution. The production database is not touched.
4.  **Update Internal State:** After the query succeeds, the `Schema Manager` is notified. It re-introspects the `users` table on the `Fresh DB` (or simply updates its model) to know that the "virtual" schema now includes the `phone` column. This ensures future rewritten queries are correct.

## 4. Implementation Phases

1.  **Phase 1: Foundation & Configuration**
    *   Solidify the configuration loading and `pgxpool` connection management.
2.  **Phase 2: Schema Introspection (`Schema Manager`)**
    *   Implement the logic to connect to a database and extract a structured list of tables and their columns by querying `information_schema`.
3.  **Phase 3: Automated FDW Setup (`FDW Manager`)**
    *   Implement the logic that takes the schema model from Phase 2 and generates the necessary `CREATE SERVER`, `CREATE USER MAPPING`, and `CREATE FOREIGN TABLE` DDL strings.
    *   Add the startup logic to execute this DDL on the fresh database.
4.  **Phase 4: Query Rewriting (`Query Rewriter`)**
    *   This is the most complex phase. Start with simple `SELECT` statements.
    *   Implement the logic to parse an incoming query and construct the `UNION ALL ... WHERE id NOT IN (...)` query.
    *   Initially, this can focus on queries to a single table.
5.  **Phase 5: Full Integration**
    *   Combine all components into the main `psql-wire` handler.
    *   Route DDL statements to the fresh database and trigger schema updates.
    *   Route `SELECT` statements through the `Query Rewriter`.
6.  **Phase 6: Advanced Features & Testing**
    *   Add support for rewriting queries with `JOINs` and aliases.
    *   Implement robust transaction handling.
    *   Write a comprehensive suite of integration tests to validate the routing and rewriting logic against actual databases.

## 5. Test Scenarios

### Initial State for All Tests:
*   **Production DB:** Contains a `users` table with `id`, `name`, `email` and 10 rows of data. Also contains an `orders` table with `order_id`, `user_id`, `amount`.
*   **Fresh DB:** Starts with the same schema as production, but all tables are empty.
*   **Proxy:** Is running and has already set up the FDW connections.

### Scenario 1: Data Federation (The "Happy Path")

*   **Action:** `SELECT * FROM users;`
*   **Expected Result:** The query should return all 10 rows from the production database. The proxy should rewrite the query to select only from the `prod_users` foreign table, as the local `users` table is empty.

### Scenario 2: DDL Execution and Isolation

*   **Action 1:** `ALTER TABLE users ADD COLUMN phone_number VARCHAR(20);`
*   **Expected Result 1:** The query succeeds. The `users` table in the **Fresh DB** now has the `phone_number` column. The `users` table in the **Production DB** is unchanged.
*   **Action 2:** `SELECT phone_number FROM users;`
*   **Expected Result 2:** The query succeeds and returns an empty result set (as there are no users with phone numbers yet). The query should not fail.

### Scenario 3: Data Creation in Fresh DB

*   **Action:** `INSERT INTO users (name, email) VALUES ('new_user', 'new@example.com');`
*   **Expected Result:** The query is routed to the **Fresh DB**. A new user is created there.
*   **Action 2:** `SELECT name FROM users WHERE name = 'new_user';`
*   **Expected Result 2:** The query returns the single row for 'new_user' from the Fresh DB.

### Scenario 4: Data Override (Fresh Takes Precedence)

*   **Action 1:** `UPDATE users SET email = 'updated.email@example.com' WHERE id = 5;` (Assume user with ID 5 exists in production).
*   **Expected Result 1:** The query is routed to the **Fresh DB**. A new row is created in the fresh `users` table with the updated email for user 5.
*   **Action 2:** `SELECT id, email FROM users;`
*   **Expected Result 2:** The query returns 10 rows. For the user with `id = 5`, the email should be 'updated.email@example.com'. For all other 9 users, the emails should be the original ones from the production database. This validates that the `WHERE id NOT IN (...)` logic is working correctly.

### Scenario 5: Querying New vs. Old Columns

*   **Setup:** Run `ALTER TABLE users ADD COLUMN city VARCHAR(50);` and `INSERT INTO users (id, name, city) VALUES (11, 'city_user', 'New York');`
*   **Action:** `SELECT id, name, city FROM users;`
*   **Expected Result:** The query should return 11 rows. The 10 users from production should have a `NULL` value for `city`. The 'city_user' should have 'New York' for `city`.

### Scenario 6: Joins and Complex Queries

*   **Action:** `SELECT u.name, o.amount FROM users u JOIN orders o ON u.id = o.user_id WHERE u.id = 7;`
*   **Expected Result:** The query should be rewritten to join the local/fresh `users` table with the foreign `prod_orders` table (and potentially the local `orders` table if any were created). The correct order amounts for user 7 from the production database should be returned.

### Scenario 7: Transaction Handling

*   **Action:** 
    ```sql
    BEGIN;
    INSERT INTO users (name) VALUES ('transaction_user');
    UPDATE orders SET amount = 0 WHERE user_id = 1;
    COMMIT;
    ```
*   **Expected Result:** All statements within the transaction block should be routed to the **Fresh DB**. After the commit, 'transaction_user' should exist in the unified view, and the order for user 1 should be updated.
*   **Action 2:** (Similar transaction with `ROLLBACK`)
*   **Expected Result 2:** All changes should be discarded, and the data should remain as it was before the transaction began.
