# ADR-0016: Audio transcription dependency

**Status:** proposed · **Date:** 2026-10-08

## Context

Telegram voice messages arrive with `awaiting_transcription: true` and `telegram_file_id`. No transcription happens — they sit in the store waiting.

Options:
1. **Local Whisper.cpp** — Runs on device, no network, ~100MB model, slow on mobile
2. **Cloud API (OpenAI Whisper, Google Speech)** — Fast, accurate, requires network + API key, cost per minute
3. **Self-hosted Whisper (faster-whisper)** — Runs on user's server, self-controlled, needs GPU for speed

## Decision

**Option 3: Self-hosted faster-whisper** as primary, with local Whisper.cpp as fallback.

Rationale:
- Fits "local-first" philosophy (runs on user's hardware)
- No per-minute cost
- Can run on same server as PostgreSQL mirror
- faster-whisper (CTranslate2) is fast enough on CPU
- API key stays in user's control

### Implementation

New adapter: `core/api/adapters/transcription`
- Endpoint: `POST /transcribe` (multipart: audio file + language hint)
- Uses faster-whisper via Python subprocess or CGO-free Go wrapper
- Returns JSON: `{ "text": "...", "language": "tr" }`

Telegram adapter flow:
1. Receive voice message → store with `awaiting_transcription: true`, `telegram_file_id`
2. Background worker downloads file via `getFile` → sends to transcription adapter
3. On success: updates note content, removes `awaiting_transcription`

### Configuration

New env vars:
- `EVOMEM_TRANSCRIPTION_URL` — e.g., `http://localhost:8001`
- `EVOMEM_TRANSCRIPTION_MODEL` — `base`, `small`, `medium`, `large-v3`
- `EVOMEM_TRANSCRIPTION_DEVICE` — `cpu` or `cuda`

## Consequences

- New dependency: Python + faster-whisper (or Go wrapper) on server
- New service to manage (systemd/launchd)
- Transcription is async; voice notes appear first as "[transcribing...]"
- No mobile-side transcription (too heavy)