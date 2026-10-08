# SQL pool

`database/sql` is a pool, not a connection, and its defaults are unlimited.
Everything borrowed from it — rows, statements, transactions — has to be given
back on every path.

| #    | Check                                                                                                   | How                                                                    | Source                                                         |
| ---- | ------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------- | -------------------------------------------------------------- |
| SP1  | `SetMaxOpenConns`, `SetMaxIdleConns`, `SetConnMaxLifetime` and `SetConnMaxIdleTime` are set from config | read where `sql.Open` is called; defaults are unlimited, 2, none, none | Go 1.27 — database/sql DB                                      |
| SP2  | With pgxpool, `pool_max_conns` (or `MaxConns`) is set rather than left at `max(4, NumCPU)`              | read the pool configuration                                            | pgx 5.11 — pgxpool ParseConfig                                 |
| SP3  | One pool per process, created in `main` and closed on shutdown                                          | grep `sql.Open(` and `pgxpool.New`                                     | Go 1.27 — database/sql                                         |
| SP4  | `defer rows.Close()` follows every successful query                                                     | golangci-lint `sqlclosecheck`                                          | golangci-lint 2.14 — sqlclosecheck                             |
| SP5  | `rows.Err()` is checked after every iteration loop                                                      | golangci-lint `rowserrcheck`                                           | golangci-lint 2.14 — rowserrcheck; Go 1.27 — database/sql Rows |
| SP6  | Every `BeginTx` is followed by `defer tx.Rollback()`, and the commit's error is checked                 | read each transaction                                                  | Go 1.27 — database/sql Tx                                      |
| SP7  | No outbound HTTP call happens while a transaction is open                                               | read the code between `BeginTx` and `Commit`                           | Go 1.27 — database/sql Tx                                      |
| SP8  | Queries use placeholders; no `fmt.Sprintf` or `+` builds SQL from input                                 | gosec G201, G202                                                       | gosec 2.29 — G201, G202; OWASP ASVS 5.0.0 V1                   |
| SP9  | `sql.ErrNoRows` (or `pgx.ErrNoRows`) is matched with `errors.Is` and mapped to not-found                | grep `ErrNoRows`                                                       | Go 1.27 — database/sql; Go 1.27 — package errors               |
| SP10 | An idempotency key is inserted in the write's transaction under a unique constraint                     | read the create path; a replay test returns one row                    | RFC 9110 §9.2.2; IETF draft idempotency-key-header-07          |

## Why each one

**SP1** is a default that is fine on a laptop and wrong in production.
Unlimited open connections means a traffic spike becomes a connection spike,
and the database refuses connections from every service that shares it — not
only this one.

**SP4 and SP5** are two different failures. An unclosed `rows` holds its
connection until garbage collection; an unchecked `rows.Err()` turns a network
error halfway through the result set into a short, silently truncated list.

**SP6** costs one line and removes a whole category. With `defer tx.Rollback()`
directly after `BeginTx`, every early return and every panic releases the
transaction; after `Commit` the rollback is a no-op.
