# ADR-0027: The server can group notes itself, with a key somebody typed into the panel

**Status:** accepted · **Date:** 2026-10-09 · **Amends:** ADR-0023

## Context

ADR-0023 decided that grouping happens one way: an agent connected over MCP
reads notes and writes clusters. It considered **evomem calling a model
service** and rejected it, in these words: it "would organise on a timer with
nobody present, and it would put the cost, the key and the privacy of every
note in the catalog into an operator's hands."

The direction given since is explicit and it includes both paths:

> sunucuda ki kümüle işlemlerini mcp ile bağlandığım yapay zeka platformu örn
> cloude **yada panelden girdiğim yapay zeka api si ile direk sunucuda**
> yapılması

So the rejected option is wanted. This record says what changes, what does
not, and what the first rejection was actually right about.

## Decision

### 1. Evomem may call a model, when a person has given it a key

The MCP path is untouched and stays the default: an agent that is already
connected groups notes with the tools from ADR-0023. This adds a second
path for when no agent is there.

ADR-0023's objection had two halves and they do not survive equally:

- **"organise on a timer with nobody present"** — this was right, and it is
  kept. There is no timer. Grouping runs when somebody presses a button, the
  same rule ADR-0025 set for pulling. A run is one request, started by a
  person, and it says what it did.
- **"the key, the cost and the privacy in an operator's hands"** — this was
  an argument against *an operator deciding for the user*. The person who
  types the key into the panel is the user, and they are deciding for
  themselves. With no key, the feature is off and nothing leaves the machine:
  the same switch `EVOMEM_TRANSCRIPTION_URL` is for transcription (ADR-0016).

**What this costs, plainly:** with a key configured, pressing the button
sends the text of the notes being grouped to whatever server the key points
at. That is the feature. The panel says so at the point the key is entered,
and the note content is what travels — not a summary, not a hash.

### 2. The interface is OpenAI-compatible chat completions

`POST {base}/v1/chat/completions`, `Authorization: Bearer <key>`, the same
shape transcription targets for the same reason: one documented interface
that OpenAI, Ollama, llama.cpp, LM Studio and vLLM all answer, so the key
can point at a laptop as easily as at a vendor.

Verified against OpenAI's own OpenAPI document (`openai/openai-openapi`,
`openapi.yaml`, `info.version: 2.3.0`, read 2026-10-09), not from memory:

- the request requires `model` and `messages`; a message is `{role, content}`
- `response_format: {"type": "json_object"}` is the older JSON mode, and its
  own description says the model "will not generate JSON without a system or
  user message instructing it to do so"
- the reply carries the text at `choices[].message.content`, which the schema
  allows to be null, and a reason at `choices[].finish_reason`
- `max_tokens` is **deprecated** in favour of `max_completion_tokens`
- the security scheme is HTTP bearer

**`response_format` is sent but never relied on.** A self-hosted server may
not know the field. The prompt asks for JSON regardless — which the spec
requires anyway — and the reply is parsed by finding the first JSON object in
the text. So a server that ignores the field still works, and one that
rejects unknown fields is the only case that fails.

**`max_completion_tokens` is what gets sent**, being the field the current
spec blesses. Older compatible servers that only know `max_tokens` will
ignore it and use their own default. Accepted: an ignored ceiling is a cost
risk at a vendor, and a wrong field name is a 400 at home.

### 3. The key lives in the keyring that already exists

`connections` holds sealed credentials (ADR-0025), and a model key is a
sealed credential. It goes in the same table with `source_type = "model"`,
sealed with the same `EVOMEM_SECRET_KEY`, listed by the same panel, removed
by the same button. No second table, no second seal, no second UI.

Two consequences follow and both are written down rather than hidden:

- **`connect.Run` must skip it.** A model row is not a source and has no
  puller; left alone, every `pull-sources` run would mark it failed with
  "nothing here knows how to pull that source". It is skipped by name, with
  the reason in the code, so a genuinely unknown source type is still
  reported.
- **`query` carries the model name.** The column means "what we ask this
  source for", and for a model that is which model. No caller reads the field
  directly: `Connection.ModelName()` does, so the overload has one site.

### 4. A run groups what is in no cluster, and never regroups

The notes offered to the model are the project's notes that belong to no
cluster, oldest first, capped. A note already in a group is left alone.

This is the smallest rule that is safe to press twice. The alternative —
hand over everything and let the model rearrange — makes the button
destructive: a second press could dissolve groups a person made or an agent
made, and nobody pressed it for that.

**The cap is 200 notes and 2000 characters per note.** A catalog larger than
that takes several presses. An uncapped run is an unbounded bill and a
request large enough to be refused.

### 5. The model's answer is filtered, not trusted

The reply names notes by id. Only ids that were in the request are kept; an
id the model invented or remembered from somewhere else is dropped and
counted. A group left with no valid notes is not created.

This is not defence against a malicious model. It is that a model asked for
ids returns ids that look right, and a cluster naming a note that does not
exist — or worse, one from another project — is a grouping nobody can read.

### 6. Groups go straight in, like ADR-0023's

No review queue, for the reason ADR-0023 gave: a cluster is a label over
notes that are already there, it changes nothing a note says, and deleting it
undoes it. `evomem clusters` shows what happened and `-delete` reverses it.

## Consequences

### Costs accepted

- **Note text now leaves the machine on a button press**, when a key is
  configured. Local-first survives only in the sense that the switch is off
  by default and the key is the user's own. This is the largest cost in the
  record and it is the thing the feature is.
- **A stolen `evomem.db` plus `EVOMEM_SECRET_KEY` is now a model key too.**
  The keyring was already worth that much; it is worth more now.
- **The grouping is only as good as one prompt.** There is no evaluation, no
  second pass, and no measure of whether the groups are any good. The person
  reading `evomem clusters` is still the measure (ADR-0023).
- **Cost is unbounded per press.** 200 notes of 2000 characters is a real
  request. Nothing here counts tokens or stops at a budget.
- **`connections` now holds two kinds of row** and tells them apart by a
  string. A third kind would be the point to stop and give it a column.

### Explicitly not decided here

- **Anthropic's own Messages API.** Reaching Claude through this endpoint
  needs a compatibility layer in front. Adding a second implementation behind
  an interface is a later decision, not a shape this closes off.
- **Re-grouping.** Nothing here improves a grouping that came out wrong; the
  only tools are delete and press again.
- **Budgets, rate limits or token accounting.**
- **Clusters still do not sync** (ADR-0023), so a grouping made here stays on
  the machine that made it.
