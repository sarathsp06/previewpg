# End-to-End Tests

This directory contains the end-to-end (E2E) tests for the SQL Federation Proxy. These tests are designed to verify the complete functionality of the proxy in a realistic environment.

## Test Environment

The E2E tests use `dockertest` to spin up the following containers for each test run:

1.  A **Production Database** (`postgres:16-alpine`): This container simulates the production database.
2.  A **Fresh Database** (`postgres:16-alpine`): This container simulates the fresh/development database.
3.  The **SQL Federation Proxy**: The proxy itself is run as a Go application within the test process.

The `TestMain` function in `main_test.go` manages the lifecycle of these containers, ensuring they are started before any tests are run and cleaned up afterwards.

## Tested Functionality

The E2E tests currently cover the following scenarios:

### 1. FDW Setup and Schema Introspection (`TestFDWSetup`)

This test verifies the proxy's ability to correctly set up the Foreign Data Wrapper (FDW) environment upon startup.

**Test Steps:**

1.  A `users` table is created in the **Production Database**.
2.  The **Proxy** is started.
3.  The test verifies that the proxy performs the following actions on the **Fresh Database**:
    *   Creates the `postgres_fdw` extension.
    *   Creates a foreign server named `prod_server` that points to the production database.
    *   Creates a user mapping for the current user.
    *   Introspects the production database schema and identifies the `users` table.
    *   Creates a foreign table named `prod_users` in the fresh database that corresponds to the `users` table in the production database.

**Success Criteria:**

*   The test passes if the `prod_users` foreign table is found in the `pg_foreign_table` catalog of the fresh database.
*   The proxy starts without errors.
*   A connection can be successfully established to the proxy.
