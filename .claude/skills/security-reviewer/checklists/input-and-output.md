# Input and output

Encode at the sink, parameterise at the query, validate against a schema at the
edge. Most of this the framework does; the review is about the places it does
not.

| #   | Check                                                                                   | How                                                  | Control                           |
| --- | --------------------------------------------------------------------------------------- | ---------------------------------------------------- | --------------------------------- |
| IO1 | Every query is parameterised; no string concatenation reaches a database                | grep for raw SQL and interpolation                   | ASVS 5.0 — injection              |
| IO2 | Input is validated against a schema at the edge, allow-list not deny-list               | find the validation; check it rejects unknown fields | ASVS 5.0 — validation             |
| IO3 | Mass assignment is impossible: the request model is not the storage model               | compare the bound type with the entity               | API3:2023                         |
| IO4 | Output is encoded for its sink — HTML, attribute, URL, shell, SQL — at the point of use | find one place that renders user data                | ASVS 5.0                          |
| IO5 | No deserialisation of untrusted data into arbitrary types                               | grep the deserialisers for type-name handling        | ASVS 5.0                          |
| IO6 | Uploads are limited by size and type, stored outside the web root, and never executed   | read the upload handler                              | ASVS 5.0                          |
| IO7 | A path built from input cannot escape its directory                                     | grep for path joins on input                         | ASVS 5.0                          |
| IO8 | Expensive endpoints are rate limited or paginated; nothing is unbounded                 | find the list endpoints and the report endpoints     | API4:2023                         |
| IO9 | Model output is never executed, and never treated as an instruction                     | for LLM code paths, find where the output goes       | OWASP LLM 2025 — prompt injection |

## Why each one

**IO3** is the one that looks like a convenience. Binding the request straight
onto the entity means every property is settable by the caller, including the
one that says which tenant the row belongs to.

**IO8** is a denial of service that needs no attacker and no malice — one
customer with more rows than the others produces it on a Tuesday.

**IO9** is the newest and the least intuitive: a model's output is data that an
attacker may have influenced through its input. Treating it as a command is the
same class of mistake as `eval` on a request body.
