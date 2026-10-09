# CI signal

A red build nobody acts on is worse than no build, because it teaches everybody
that red does not mean anything.

| #   | Check                                                                              | How                                                   | Source                                        |
| --- | ---------------------------------------------------------------------------------- | ----------------------------------------------------- | --------------------------------------------- |
| CI1 | A failure message says what was expected and what happened                         | read three recent failures without opening the files  | —                                             |
| CI2 | The commit-gate suite finishes in a time people will wait for                      | time it; beyond roughly ten minutes it gets skipped   | —                                             |
| CI3 | Nothing is skipped without an issue and an expiry                                  | count the skips and read their reasons                | A permanent skip is a deleted test with noise |
| CI4 | The build is red only for real failures                                            | look at the last twenty runs; how many reds were real | Alert fatigue                                 |
| CI5 | Output on success is quiet                                                         | read a green run's log                                | Noise on green is noise nobody reads on red   |
| CI6 | The failing test is identifiable from the summary, without downloading an artifact | read the summary of a failed run                      | —                                             |
| CI7 | A flaky test is quarantined the day it is noticed, not left failing                | read the quarantine list and its dates                | Section 2 of the skill                        |
| CI8 | The suite runs on the merge result, not only on the branch                         | read the trigger configuration                        | Semantic merge conflicts                      |

## Why each one

**CI4** is the one that decides whether any of the others matter. A pipeline
that is red half the time has no gate — people learn to merge through it, and
then the one real failure goes through with everything else.

**CI2** is not about impatience. A gate people skip is not a gate, and the
decision to skip it is made silently by everybody, once, and never revisited.

**CI8** catches the failure that no branch build can: two changes that are each
correct and together are not. If CI only ever tests the branch, that class of
failure reaches the default branch by construction.
