# Trust boundaries

Run this before reading any code. Most findings that matter are visible here
and invisible in a grep.

| #   | Check                                                                                                                    | How                                                              | Control                         |
| --- | ------------------------------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------- | ------------------------------- |
| B1  | Every boundary is named: network edge, database, broker, each third-party API, filesystem, CI runner, agent tool surface | list them; a boundary nobody named is a boundary nobody reviewed | ASVS 5.0 — architecture         |
| B2  | For each boundary, what crosses it is written down **in both directions**                                                | read the adapters, not the docs                                  | ASVS 5.0 — architecture         |
| B3  | Data leaving is reviewed as carefully as data arriving                                                                   | what does this send to a third party, and does the user know     | A01:2025 Broken Access Control  |
| B4  | Each boundary names who the caller is and how that is established                                                        | follow one request from the edge inwards                         | ASVS 5.0 — authentication       |
| B5  | Validation happens on the trusted side of each boundary, not only at the client                                          | find the server-side check for one client-side rule              | ASVS 5.0 — validation           |
| B6  | A server-side request to a caller-supplied URL is either impossible or allow-listed                                      | grep for outbound calls built from input                         | A01:2025, which absorbed SSRF   |
| B7  | The tool surface of any agent or MCP server is enumerated, with what each tool can reach                                 | read the tool registrations                                      | OWASP Agentic Applications 2026 |

## Why each one

**B3** is the one nobody runs. Reviews look inwards at what an attacker sends
and skip what the system volunteers — to a logging provider, an analytics
script, an LLM prompt, an error tracker. The data leaves either way.

**B6** moved: server-side request forgery is no longer its own Top 10 entry,
it was absorbed into broken access control in the 2025 edition. Cite it there.

**B7** is the newest boundary and the least reviewed. A tool that takes a path
and reads it is a file-read primitive handed to whatever can talk to the model.
