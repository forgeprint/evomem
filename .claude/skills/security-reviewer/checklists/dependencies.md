# Dependencies and the build

Software supply chain failures became their own Top 10 category in the 2025
edition. This is the checklist most often skipped, and the one where a single
finding has the widest blast radius.

| #   | Check                                                                          | How                                  | Control                                 |
| --- | ------------------------------------------------------------------------------ | ------------------------------------ | --------------------------------------- |
| P1  | Every dependency is pinned; no floating range on a direct dependency           | read the manifest and the lockfile   | A03:2025 Software Supply Chain Failures |
| P2  | A lockfile exists and is committed                                             | it is in the repository              | A03:2025                                |
| P3  | CI actions and base images are pinned by digest or commit SHA, not by tag      | read the workflow and the Dockerfile | SLSA v1.2; CIS Docker Benchmark         |
| P4  | The build token has the least privilege that works, and is scoped per job      | read the permissions block           | SLSA v1.2                               |
| P5  | A fork's pull request cannot reach a secret                                    | read the workflow triggers           | SLSA v1.2                               |
| P6  | No install step pipes a download into a shell                                  | grep the recipe and the workflow     | NIST SSDF; rule 20                      |
| P7  | Transitive dependencies are enumerated at least once, and the risky ones named | run the tree; read it                | A03:2025                                |
| P8  | A published artifact carries provenance where the ecosystem supports it        | check the publish step               | SLSA v1.2                               |
| P9  | There is a documented way to report a vulnerability, and a response target     | `SECURITY.md` exists and says both   | NIST SSDF — respond                     |

## Why each one

**P3** is the one that reads as pedantry and is not. A tag is a pointer
somebody else can move. Pinning an action by tag means a third party can change
what runs inside your build, with your token, after your review.

**P5** is the specific failure that turns a public repository into a credential
leak: a workflow that runs on a fork's pull request with access to secrets is
an arbitrary-code-execution path open to anybody with a GitHub account.

**P9** is not paperwork. Without it, somebody who finds a real problem either
posts it publicly or gives up, and both of those are worse than an inbox.
