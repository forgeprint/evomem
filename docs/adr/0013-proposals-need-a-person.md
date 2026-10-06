# ADR-0013: An agent proposes, a person accepts

**Status:** accepted · **Date:** 2026-10-06
**Amends:** [ADR-0008](0008-read-only-tools.md), which made the tool set
read-only and left this question to Phase 4.

## Context

ADR-0008 shipped four read-only MCP tools and said that whether an agent
should be able to write — and if so, how — was a decision to take alongside
the ingestion adapters. Phase 4 is done and the question was still open.

Three options:

1. **Nothing.** Writing stays `evomem add` and `POST /ingest`. Honest, but a
   memory an agent cannot add to is one a person has to transcribe for it, and
   the thing worth remembering is usually discovered mid-task.
2. **Direct write.** One tool, straight into `notes`. Immediate, and it makes
   the store contents something nobody chose: a model that is wrong, or that
   is repeating something it read on a web page, writes it anyway — into the
   same store the MCP tools then read back as context. ADR-0009 exists because
   third-party text reaching a model is a hazard; a direct write tool makes
   the store itself the channel.
3. **Propose and review.** The agent suggests, a person decides, and only the
   decision creates a note.

The user chose 3.

## Decision

One new tool, `propose_note`, and a new command, `evomem review`.

A proposal goes into its own `proposals` table. Nothing reads it but
`evomem review`: it is not searched, not listed, not pushed to the mirror, and
not readable by `get_note`. `AcceptProposal` is the only path from a proposal
into `notes`, and the only caller is a person at a terminal.

The tool says so twice — `"remembered": false` in `structuredContent`, and in
the text, because a model that believes it has written to memory will tell the
user it has.

## Consequences

- The tool set is no longer read-only, but memory still is: nothing an agent
  does puts a note in front of the next session.
  `TestProposeNoteIsNotMemory` and `TestAProposalIsNotANote` are what hold
  that, checked through the tools and through the store.
- **A person is now in the loop, which means a person can become the
  bottleneck.** A queue nobody reads is a feature nobody has. `sync-status`
  reports what is waiting, and `review` prints the exact command to accept the
  first one.
- **The queue is bounded at 200 pending.** An agent in a loop can propose
  without limit, and a review queue with ten thousand entries is not a review
  queue. Going over is a tool error that tells the model to stop rather than
  to rephrase.
- **A duplicate while pending returns the existing proposal.** A retried call,
  or the same realisation twice in one conversation, must not lengthen the
  queue. After a decision the same content may be proposed again: a rejection
  is not a permanent ban.
- **An accepted note is not marked tainted, even if the proposal was.** A
  person read it and said yes, and that endorsement is exactly what the mark
  exists to be absent for. What the agent claimed is kept as
  `proposed_tainted`, alongside `proposed_by` and `proposal_id`, so the
  provenance survives the decision without being mistaken for the live mark.
- **`tainted` on a proposal is self-declared.** An agent that read a web page
  is the only party that knows it did. This is weak on its own; the review
  step is what the property actually rests on, which is why
  `evomem review` prints the claim where the person will read it.
- **No accept-with-edit.** Accept as proposed, or reject and write your own
  with `evomem add`. Editing on accept would blur whose words are in the
  store, and the provenance metadata would be claiming something untrue.
- Decided proposals are kept — so the same thing is not proposed again
  tomorrow, and so there is a record of what was refused — and archiving
  clears the old ones. A **pending** proposal is never archived whatever its
  age: nobody has looked at it, and a queue that silently drops its oldest
  entries is worse than a long one.
- `proposed_by` comes from the MCP client's own `clientInfo`. It is a label
  for the person reviewing, never a credential: a client says what it likes,
  and nothing is decided by it.
