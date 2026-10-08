# Logging and errors

Mishandling of exceptional conditions is new to the 2025 Top 10. Two failures
live here and they pull in opposite directions: the logs say too much to the
wrong reader, and the system says too little to the person holding the pager.

| #   | Check                                                                                                  | How                                            | Control                                        |
| --- | ------------------------------------------------------------------------------------------------------ | ---------------------------------------------- | ---------------------------------------------- |
| E1  | No secret, token, connection string, password or full request body is logged                           | read the auth and infrastructure log calls     | A02:2025; §5b                                  |
| E2  | Personal data in logs is minimised and has a retention answer                                          | read what is logged at the edge                | Privacy regulation; ASVS 5.0                   |
| E3  | The error returned to a caller carries no stack trace, no query, no internal host                      | read the exception handler                     | A10:2025 Mishandling of Exceptional Conditions |
| E4  | Failures are not swallowed: nothing catches broadly and continues as if nothing happened               | grep for empty or log-only catch blocks        | A10:2025                                       |
| E5  | A security-relevant event is logged: authentication failure, authorization denial, privilege change    | find one of each                               | ASVS 5.0 — logging                             |
| E6  | Log entries carry enough to correlate — trace id, user id where lawful, and the time in UTC            | read one entry end to end                      | ASVS 5.0                                       |
| E7  | An error path cannot be used as an oracle: a wrong password and an unknown user look and take the same | compare both paths                             | ASVS 5.0 — authentication                      |
| E8  | Logs cannot be forged by input: newlines and control characters in user data are neutralised           | grep for user data concatenated into a message | ASVS 5.0 — log injection                       |

## Why each one

**E4** is the one the 2025 edition added a category for, and it is the quietest
failure in the list. A broad catch that logs and continues turns a failed
authorization check into a successful request — the system keeps going, having
skipped the step that mattered.

**E7** is why "invalid username or password" is one message and not two. It is
also why the two paths must take the same _time_: an early return on an unknown
user enumerates accounts as effectively as a different message.

**E8** is small until logs are the evidence. A newline in a username writes a
second log line, and a reviewer reading the file cannot tell which lines the
system wrote.
