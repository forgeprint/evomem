# Behaviour, not implementation

A suite that breaks when you refactor is not thorough. It is coupled to the
code rather than to what the code is for, and it will be deleted the first time
somebody needs to move fast.

| #   | Check                                                                 | How                                                              | Source                        |
| --- | --------------------------------------------------------------------- | ---------------------------------------------------------------- | ----------------------------- |
| B1  | Every test name is a sentence about behaviour                         | read ten at random; can you tell what the system does            | Test naming as specification  |
| B2  | Assertions are on observable outcomes, not on which method was called | grep for mock verification as the only assertion                 | Test doubles — mocks vs stubs |
| B3  | One concept per test; two unrelated outcomes are two tests            | look for tests asserting on unrelated things                     | —                             |
| B4  | Arrange, act and assert are visible                                   | read one test; the three parts should be obvious                 | AAA                           |
| B5  | No test asserts on a private method or an internal field              | grep for reflection and visibility-relaxing attributes           | —                             |
| B6  | Framework behaviour is not tested                                     | look for tests of routing, ORM persistence, attribute validation | —                             |
| B7  | A refactor that preserves behaviour leaves the suite green            | rename a class, extract a method, run it                         | The definition of the rule    |
| B8  | Every fixed bug has a test that fails without the fix                 | for one recent fix, revert it and watch the test fail            | Regression testing            |

## Why each one

**B7 is the check.** Everything else in this file is a proxy for it, and it is
the only one that measures the property directly. Rename a class and extract a
method — nothing about the system's behaviour has changed, so nothing in the
suite should care.

**B2** is the most common violation, and it is comfortable because it is easy.
Asserting that a repository's `Save` was called proves that the code called
something. Asserting that the row exists proves it worked.

**B8** has a specific procedure that people skip: write the test **first**, and
watch it fail, before the fix. A regression test written after the fix has
never demonstrated that it can fail, so it has never demonstrated anything.
