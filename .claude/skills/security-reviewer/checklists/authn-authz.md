# Authentication and authorization

Three of the top five API risks are authorization failures, and the most
common single API vulnerability is object-level. This is where a review earns
its keep.

| #   | Check                                                                                                           | How                                                                              | Control                       |
| --- | --------------------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------- | ----------------------------- |
| AA1 | Every endpoint is authenticated unless it is deliberately public, and the public list is short and written down | enumerate the endpoints; find the default                                        | ASVS 5.0 — access control     |
| AA2 | **Every read and write of an object checks that this caller may touch _that_ object**                           | for one resource, find the ownership check; then find the endpoint that lacks it | API1:2023 BOLA                |
| AA3 | Authorization is decided server-side, never from a field in the request                                         | grep for a role or tenant id read from the body or a header                      | API5:2023 BFLA                |
| AA4 | The tenant discriminator comes from the token, not from the URL or the body                                     | follow it from the token to the query                                            | API1:2023                     |
| AA5 | Administrative functions are separated and separately authorized                                                | list them; check they are not merely unlisted                                    | API5:2023                     |
| AA6 | Tokens expire, and there is a revocation story                                                                  | read the issuance and the validation                                             | ASVS 5.0 — session management |
| AA7 | Password storage, if any, uses a current memory-hard algorithm with per-user salt                               | find the hashing call                                                            | ASVS 5.0 — cryptography       |
| AA8 | Authentication failures are rate limited and do not reveal which half was wrong                                 | read the failure path and its message                                            | ASVS 5.0; API4:2023           |
| AA9 | A JWT's algorithm, issuer, audience and expiry are all validated                                                | read the validation parameters, not the issuance                                 | ASVS 5.0                      |

## Why each one

**AA2 is the finding.** It is invisible to every scanner, it survives code
review because the code looks right, and it is exploitable by anybody who has
an account and can change a number in a URL. If you check one thing on an API,
check this, and check it by finding the endpoint that _lacks_ the check rather
than the one that has it.

**AA4** is AA2's structural version. A tenant id that can arrive from the
request is not a boundary, it is a suggestion — and it turns one careless
endpoint into a cross-tenant read.

**AA9** catches the classic: careful issuance, and validation that accepts any
issuer because the parameters were left at their defaults.
