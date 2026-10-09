# Test data and doubles

What you fake decides what the suite can catch. Fake the wrong thing and the
tests pass forever, including through the outage.

| #   | Check                                                                                 | How                                                        | Source                                         |
| --- | ------------------------------------------------------------------------------------- | ---------------------------------------------------------- | ---------------------------------------------- |
| TD1 | Nothing you do not own is mocked                                                      | list the mocks; which are third-party clients              | Do not mock what you do not own                |
| TD2 | Time, randomness, payments and outbound mail **are** faked                            | check they are injectable                                  | Slow, costly or non-deterministic              |
| TD3 | Database tests run against the real engine, in a container                            | read the fixture; an in-memory substitute is the finding   | Different constraint and transaction semantics |
| TD4 | Each test builds the data it needs; there is no shared golden fixture everybody edits | look for one large seed file                               | Coupling through data                          |
| TD5 | Builders have sensible defaults, and a test sets only what it is about                | read a test's arrange block; is the relevant value obvious | Object mother / builder                        |
| TD6 | No real credentials, tokens or production data anywhere in the suite                  | grep; check the fixtures and the recorded interactions     | §5b                                            |
| TD7 | Recorded third-party interactions have a date and a refresh story                     | read the cassettes                                         | A recording is a snapshot that ages            |
| TD8 | Test data is cleaned up, or the database is fresh per test                            | read the teardown                                          | Order dependence                               |

## Why each one

**TD1 and TD2 are the same decision made twice**, in opposite directions, and
getting them backwards is the classic mistake. A mock of a payment provider's
client is your belief about their API frozen in amber; a real clock in a test
is a failure waiting for a slow machine. Fake what you own the boundary of;
test against the real thing where you do not.

**TD3** is specific because the substitution is so tempting. An in-memory
database is faster and it has different semantics for constraints,
transactions, collation and case sensitivity — which is the exact list of
things the test was going to catch.

**TD4** is how a suite becomes unmaintainable without anybody deciding to. One
shared fixture that every test depends on cannot be changed by anybody, so it
only grows.
