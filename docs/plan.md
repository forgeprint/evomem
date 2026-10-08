# plan.md - Evomem Project Implementation Roadmap

This step-by-step roadmap governs the iterative development of the Evomem Monorepo. Execute each phase sequentially. Verify functionality with explicit tests before proceeding.

## Phase 1: Monorepo Foundation & Core Go SQLite Storage
- [x] Initialize the workspace structure:
  ```text
  evomem/
  ├── apps/
  │   ├── web/
  │   └── mobile/
  ├── core/
  │   ├── api/
  │   └── mcp/
  ├── shared/
  │   ├── models/
  │   └── database/
  ├── docs/
  └── README.md
  ```
- [x] Initialize the root Go module: `go mod init github.com/forgeprint/evomem`
- [x] Integrate a pure-Go SQLite package (e.g., `modernc.org/sqlite` or pure Go `sqlite3` port) with `CGO_ENABLED=0` capability.
- [x] Write database initialization logic creating the `notes` table and the `notes_fts` FTS5 virtual table for lightning-fast text searches:
  ```sql
  CREATE TABLE notes (
      id TEXT PRIMARY KEY,
      project_id TEXT NOT NULL,
      content TEXT NOT NULL,
      source_type TEXT NOT NULL,
      created_at DATETIME NOT NULL,
      updated_at DATETIME NOT NULL,
      metadata TEXT DEFAULT '{}'
  );
  CREATE VIRTUAL TABLE notes_fts USING fts5(id, content, content='notes');
  CREATE INDEX idx_notes_project ON notes(project_id);
  ```
- [x] Implement data models (`shared/models`) maps to Go structures using standard `json.RawMessage` or `map[string]any` for `Metadata`.
- [x] Implement core repository CRUD queries (Create, Read, Search via FTS5, Delete) inside `shared/database` and validate completely with automated unit tests (`go test`).

## Phase 2: Core Model Context Protocol (MCP) Server
- [x] Establish standard input/output (stdio) communication protocol scaffolding under `core/mcp`.
- [x] Wire up the official or compliant lightweight Go MCP server framework. *(No framework: written against the specification by hand, under the no-new-dependencies rule. See ADR-0007.)*
- [x] Implement the `search_notes` tool capability allowing LLMs to search global text fragments using the underlying SQLite FTS5 index.
- [x] Implement the `get_project_context` tool filtering and delivering historical note payloads bounded by a specific `project_id`.
- [x] Test the local server locally with the Claude Desktop configuration file or terminal mocking tools.
- [x] Implement `get_note`, `propose_note`, `list_projects` tools.
- [x] Wire MCP server into `evomem mcp` CLI command.

## Phase 3: Local-First Flutter Mobile App
- [x] Initialize clean Flutter environment inside `apps/mobile` utilizing Dart.
- [x] Configure local embedded storage using a solid local-first package matching the Go schema architecture (e.g., `sqflite` or `drift`).
- [x] Build minimalist user interfaces for swift text capturing.
- [ ] Implement background-ready audio recording module saving raw files locally to native storage directories and appending references to the local DB.
      **Bu kutu 2026-10-08'e kadar yanlış olarak işaretliydi.** Mobilde ses
      kaydı diye bir şey yok: ne paket, ne mikrofon izni, ne arayüz. Sesin
      sunucuya nasıl ulaşacağı ADR-0018'de kararlaştırıldı; kaydın kendisi
      henüz yazılmadı.

## Phase 4: Integration Adaptors & Ingestion Trigger Engines
- [x] Implement Telegram Ingestion Handler (`core/api/adapters/telegram`): Parse text payloads or voice metadata via Telegram Bot API webhooks and ingest them as records using `source_type: "telegram"`.
- [x] Implement Jira Integration Adapter (`core/api/adapters/jira`): Set up API ingest hooks mapping project issues to standard note blocks with high extensibility fields (e.g., storing `jira_issue_key`, `status` inside `metadata`).
- [x] Create a fast HTTP server entrypoint in Go (`core/api`) to handle fast payloads incoming via Apple Shortcuts automation triggers.
- [x] Wire HTTP server into `evomem serve` CLI command.
- [x] Implement Telegram multi-project routing (chat_id → project mapping + hashtag fallback).

## Phase 5: Cloud Synchronization Pipeline (Lokal SQLite -> Cloud PostgreSQL)
- [x] Draft a background worker pipeline checking for internet connectivity status.
- [x] Implement delta synchronization logic securely mirroring local tracking entries from SQLite over a central secure PostgreSQL node.
- [x] Implement archiving rules filtering records older than 6 months to thin down the active local SQLite database footprint on demand.
- [x] Implement `evomem pull` (delta sync from remote) and `evomem restore` (full restore from remote) CLI commands.
- [x] Add `PullNotes` and `PullAll` methods to PostgreSQL transport for Remote interface.

## Plan dışı: dağıtım
- [x] Tag ile tetiklenen release workflow'u (`.github/workflows/release.yml`):
      `scripts/release.sh` temiz runner'da çalışır, `dist/*` attest edilir,
      release **taslak** olarak açılır — yayınlama insanın işi. → ADR-0017
- [x] İlk sürüm kesildi ve yayınlandı: **v0.1.1**. (`v0.1.0` tag'i workflow'dan
      önce atılmıştı, release'i hiç olmadı; oynatılmadı, yenisi kesildi.)
      `gpg` kurulu olmadığı için tag `-s` değil `-a` ile atıldı.
- [x] Ses dökümü (`core/transcribe` + `evomem transcribe`) → ADR-0016;
      OpenAI-uyumlu harici servis, yerel yedek yok, yapılandırılmazsa kapalı
- [ ] Mobilde ses kaydının kendisi (paket, izinler, arayüz) — ADR-0018 kapsam dışı
- [ ] `POST /ingest/audio` + yerel dosya `Fetcher`'ı → ADR-0018
- [ ] Telefonun `/ingest`'ten dönen id'yi saklaması (`remote_id`); bugün atıyor,
      bu yüzden push idempotent değil → ADR-0018
- [ ] Not silindiğinde ses dosyasının da silinmesi → ADR-0018
- [ ] Dökümü onaylayıp işaretleri temizleyen akış (ayrı karar)
