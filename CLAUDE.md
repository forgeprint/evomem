# CLAUDE.md - Evomem Development Guidelines

This file defines the strict architectural constraints, coding standards, and build/test commands for the Evomem Monorepo. Claude Code must read and adhere to these guidelines during all development tasks.

## 1. Project Overview & Boundaries
* **Project Name:** Evomem (Open-source AI-Ready Memory Infrastructure)
* **Organization:** forgeprint
* **Repository Architecture:** Go + Flutter Monorepo
* **Core Philosophy:** Local-First, zero runtime dependencies for the core Go binary, low-memory footprint, and highly performant execution optimized for the local MacBook M5 hardware environment.
* **Isolation Rules:** Evomem is structurally and operationally isolated from the 'Charactly' and 'Pendra' repositories. This agent must strictly manage Evomem code base and only handle third-party context metadata belonging to external systems using the extensible fields provided.

## 2. Core Technology Stack
* **Backend & API Katmanı:** Go (Golang)
* **Memory & AI Connector:** Model Context Protocol (MCP) Server in Go
* **Mobile Layer:** Flutter (Dart)
* **Local Database:** SQLite (Pure Go driver, zero-CGO required: `modernc.org/sqlite` or equivalent)
* **Cloud Database:** PostgreSQL (Sync engine target)

## 3. Frequent Developer Commands
### Go Backend (core/api & core/mcp)
* Run all backend tests: `go test ./...`
* Build local binary: `go build -o dist/evomem ./cmd/evomem`
* Run format check: `go fmt ./...`

### Flutter Mobile App (apps/mobile)
* Fetch dependencies: `flutter pub get`
* Run development build: `flutter run`
* Execute mobile tests: `flutter test`
* Trigger code generation (if drift/freezed is used): `dart run build_runner build --delete-conflicting-outputs`

## 4. Coding & Database Standards
* **Extensible Schema Strategy:** The core SQL schema contains fixed atomic pillars (`id`, `project_id`, `content`, `source_type`, `created_at`, `updated_at`). All platform-specific or source-specific attributes (Jira fields, Telegram metrics, Apple Shortcut variables) must be stored in the generic `metadata` JSON text column.
* **Do Not Modify Schema Destructively:** Claude Code must never write breaking migrations or alter the atomic fields without user prompt confirmation.
* **SQLite Connection Management:** To prevent data locking or corruption over file systems during concurrent background writes (e.g., streaming ingestion hooks via Telegram while local MCP client reads), configure the database connection pool to restrict database updates to a single writer thread (`Single Writer` pattern) and force `PRAGMA journal_mode=WAL;`.
* **String Utilities:** Prefer compact, lexical ordered identifiers like ULID over generic UUID for primary keys to optimize clustering indexing over SQLite B-Trees.
* **Go Style Rules:** Write idiomatic, clean Go code using explicit error handling. Avoid embedding complex external third-party routing or frameworks for the internal core unless requested.
* **Never write an external API or protocol detail from memory.** Before writing code against any third-party payload shape, header name, field name, error code or protocol revision — a webhook body, a bot API, MCP, an OAuth flow — check that system's current official documentation, cite the page and the date in the code or the report, and mark anything unverifiable as unverified. A field name guessed wrong fails silently: an absent field decodes as a zero value, not an error, so the mistake surfaces as missing data weeks later rather than as a failed build.
* **Do not guess at an ambiguity.** Ask. Do not bury the question in a code comment.
* **Update `docs/ilerleme.md` at the end of every step:** what was done, what was decided, what is still open. It is the notebook between sessions, and a session that does not update it has lost the work for the next one. Internal notes (`docs/ilerleme.md`, `docs/plan.md`) are Turkish; public documents, code and comments are English.
* **Decisions are recorded, not re-argued.** Anything architectural goes in `docs/adr/` with the costs named. If a recorded decision looks unworkable, say why and wait for an answer rather than changing it.
