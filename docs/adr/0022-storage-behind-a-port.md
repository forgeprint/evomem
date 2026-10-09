# ADR-0022: Storage is a port, and the browser gets its own implementation

**Status:** accepted · **Date:** 2026-10-09

## Context

The direction changed: the note lifecycle — writing notes, and later having a
model group and organise them — is to be exercised **in a browser first**, and
the mobile app built out afterwards. More is coming in that direction: an
organising model, and MCP features that connect Claude to it.

Two things were in the way, and only the second was known.

### The web build has never run

`web/index.html` still called `_flutter.loader.loadEntrypoint` and read a
`serviceWorkerVersion` variable this Flutter no longer defines. The script
threw, the engine never started, the page was blank. CI was green throughout,
because `flutter build web --release` proves that it compiles and nothing
opens the page. Fixed separately; it is recorded here because it is the
reason nobody had noticed the next part.

### Nothing persisted, and the UI said otherwise

With the page running, adding a note showed "Note saved" and listed it. A
reload lost it. From the packages' own `pubspec.yaml` files:

| package | platforms it declares | web |
| - | - | - |
| `sqflite` | android, ios, macos | **no** |
| `path_provider` | android, ios, linux, macos, windows | **no** |
| `record` | android, ios, web, windows, macos, linux | yes |

`NotesNotifier` updates its state optimistically and writes in the
background, so a write that threw left a list that looked right.

### And the store was never read at startup, anywhere

Chasing the first two turned up a third, which is not about the web at all:
`loadNotes()` was called only from the two rollback paths. **Nothing read the
database when the app started**, on any platform, so every launch showed an
empty list over a full database. No test caught it: each one adds its notes
inside the session it then asserts on.

## Decision

### 1. A `NotesStore` port, with the DAO as its first implementation

`NotesStore` names the calls the app already makes — insert, update, delete,
read, search, the sync queue — and `NotesDao` implements it. The providers
and `SyncService` depend on the interface.

It buys nothing today: there is one implementation and it is the one that was
already there. It is the seam for what was asked for — a store backed by the
Go server, or one an organising model can reach, implements this and nothing
above it changes. Deliberately the set of calls that exist rather than a
complete SQL surface: a method nobody calls is a method the next
implementation writes for nobody.

### 2. The web keeps sqflite's API and changes what is under it

`sqflite_common_ffi_web` puts sqlite3, compiled to wasm over IndexedDB,
behind the same `DatabaseFactory`. `DatabaseHelper` swaps the factory when
`kIsWeb`, and every query, every DAO method and **every migration** is
unchanged.

The alternative was a second implementation of `NotesStore` for the browser —
rejected because it would mean two copies of the schema and two sets of
migrations to keep in step, which is how a v6 store on one platform meets a v4
on another.

### 3. `databaseFactoryFfiWebNoWebWorker`, measured rather than assumed

The package's default factory failed in the browser this was tested in:
`openDatabase` resolved to null, surfacing as `Unsupported operation:
unsupported result null`, and neither `sqflite_sw.js` nor `sqlite3.wasm` was
ever fetched — it gave up before loading either. The main-thread factory
opens the same database, fetches the wasm, and the file it writes to
IndexedDB survives a reload.

**Costs:** sqlite runs on the UI isolate, so a long query janks the frame; and
two tabs on one origin each hold a connection to a virtual filesystem that
does no locking, so both writing at once can corrupt it. Both are acceptable
for a test surface and neither is acceptable for a product; revisit when the
browser stops being somewhere to try things.

### 4. The store is read when the app starts

`NotesNotifier.build()` kicks off `loadNotes()`. Two guards came with it,
because the first attempt broke things the tests then caught:

- **A revision counter.** The startup read and an optimistic add race, and
  without it a note typed a moment after launch was wiped by a query that ran
  before it was typed.
- **`ref.mounted` before every late write to state.** A read that lands after
  the notifier is gone threw. The rollback paths had the same hazard already;
  the startup read is what made it happen.

## Consequences

### Costs accepted

- **An interface with one implementation** is weight until the second arrives.
  If the server-backed store never happens, this is a layer that earned
  nothing.
- **Two storage engines to keep working.** The schema and the migrations are
  shared, but sqlite-on-wasm is not sqlite-on-a-file: the web implementation
  is marked experimental by its own author, and a browser can fail at it in
  ways a phone cannot.
- **`path_provider` still has no web implementation.** Recording writes its
  file through it, so recording in a browser will fail even though `record`
  itself supports web. Untouched here; it is the next thing in the way of the
  browser being a full test surface.
- **The jank and the cross-tab hazard** of running sqlite on the UI isolate,
  above.

### Explicitly not decided here

- **What the second implementation is.** Server-backed, model-backed, or
  neither; this only makes it possible.
- **Recording in a browser.** Needs the file path question answered.
- **Whether CI should open the page.** It builds the web app and proves only
  that it compiles, which is how a blank page stayed green for weeks. A smoke
  check that loads it and asserts a note survives a reload would have caught
  everything in this record.
