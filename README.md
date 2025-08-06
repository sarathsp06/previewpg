# PreviewPG: The Schema-Aware SQL Federation Proxy   [![Go](https://github.com/sarathsp06/previewpg/actions/workflows/go.yml/badge.svg)](https://github.com/sarathsp06/previewpg/actions/workflows/go.yml)
PreviewPG is a transparent, intelligent PostgreSQL proxy that virtualizes a preview/QA database environment. It automatically federates data between a "production" and a "fresh" database, presenting a unified, consistent view to the client.

## Overview

The goal of PreviewPG is to allow developers and QA engineers to work with a database that feels like a complete, up-to-date copy of production, but with the ability to make schema changes and add new data without affecting the actual production environment. It achieves this by acting as a proxy that intelligently rewrites queries and sends them to the appropriate database.

## Core Architecture

The proxy is a single Go application with several key internal components:

*   **Proxy Core:** Listens for client connections, accepts raw SQL queries, and streams results back.
*   **Configuration Manager:** Loads connection strings and server settings from a `config.yaml` file.
*   **Connection Manager:** Manages robust connection pools to the backend "production" and "fresh" databases.
*   **Schema Manager:** Introspects the schemas of both databases to build an in-memory model of the tables and columns.
*   **FDW Manager:** Automatically configures the "fresh" database to use PostgreSQL's Foreign Data Wrappers (`postgres_fdw`) to access data from the "production" database.
*   **Query Rewriter:** The core of the proxy. It intercepts incoming queries and rewrites them to fetch data from both the "fresh" and "production" databases, as needed.

## How it Works

1.  **Initialization:** On startup, the proxy connects to both databases, inspects their schemas, and automatically sets up the necessary foreign tables in the "fresh" database to mirror the "production" database.
2.  **Query Rewriting:** When a `SELECT` query is received, the proxy rewrites it to first query the "fresh" database, and then query the "production" database for any data that doesn't exist in the "fresh" database. This is done using a `UNION ALL` query with a `WHERE id NOT IN (...)` clause to avoid duplicate data.
3.  **DDL and DML:** `ALTER TABLE`, `INSERT`, `UPDATE`, and `DELETE` queries are sent directly to the "fresh" database, leaving the "production" database untouched.
4.  **Unified View:** The result is that the client sees a single, unified database that combines the data from both the "production" and "fresh" databases, with the "fresh" data taking precedence.

## Getting Started

To get started with PreviewPG, you will need to have Go installed on your system. You will also need to have two PostgreSQL databases accessible to the proxy: one for "production" and one for "fresh" data.

1.  **Clone the repository:**

    ```
    git clone https://github.com/your-username/previewpg.git
    cd previewpg
    ```

2.  **Configure the proxy:**

    Create a `config.yaml` file in the `config` directory, using the `config.example.yaml` file as a template. Fill in the connection details for your "production" and "fresh" databases.

3.  **Build and run the proxy:**

    ```
    go build ./cmd/proxy
    ./proxy
    ```

4.  **Connect to the proxy:**

    You can now connect to the proxy using any PostgreSQL client, using the host and port specified in your `config.yaml` file.

## Contributing

We welcome contributions to PreviewPG! If you would like to contribute, please follow these steps:

1.  Fork the repository.
2.  Create a new branch for your feature or bug fix.
3.  Make your changes and commit them with clear, descriptive messages.
4.  Push your changes to your fork.
5.  Create a pull request to the main repository.

## License

PreviewPG is licensed under the MIT License. See the `LICENSE` file for more information.
