# Coverage that means something

Coverage is a map of what the tests executed, not of what they checked. A suite
can execute every line and assert nothing at all.

| #   | Check                                                            | How                                                          | Source                                         |
| --- | ---------------------------------------------------------------- | ------------------------------------------------------------ | ---------------------------------------------- |
| CV1 | No coverage percentage is set as the goal                        | read the CI configuration and the team's definition of done  | Goodhart on a measure                          |
| CV2 | Coverage is read to find what is untested, then judged           | look at the uncovered list; is anything on it a decision     | —                                              |
| CV3 | Error paths and failure handling are covered                     | find the catch blocks and the failure branches in the report | Uncovered error handling is the expensive kind |
| CV4 | Every branch that encodes a decision is covered both ways        | branch coverage, not line coverage, on the business logic    | —                                              |
| CV5 | Mutation testing has been run at least once on the core          | run it; expect an unpleasant surprise                        | Mutation testing                               |
| CV6 | Deliberately excluded code is excluded explicitly, with a reason | read the exclusions                                          | An unexplained exclusion is a hidden gap       |
| CV7 | Breaking something on purpose makes the suite red                | invert a condition; delete a validation; run it              | The only direct measurement                    |

## Why each one

**CV7 is the whole file in one command.** Everything above it is a proxy.
Delete a validation rule, run the suite, and see what happens: a suite that
stays green has told you what its coverage number is worth.

**CV1** is not anti-measurement. It is that a threshold is reached by testing
what is easy to test, which is the code that was already obvious, and the
uncovered remainder is then the code that most needed it.

**CV5** is worth doing once even if never again. The first mutation run on a
codebase people are proud of is the most useful unpleasant afternoon available.
