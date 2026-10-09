# Forms and errors

A form is where an inaccessible interface stops being inconvenient and starts
blocking somebody from signing up, paying or getting help. Submit every form
empty, then wrong, then right, with the keyboard and with the screen reader.

| #   | Check                                                                                                         | How                                                                          | Source                                                  |
| --- | ------------------------------------------------------------------------------------------------------------- | ---------------------------------------------------------------------------- | ------------------------------------------------------- |
| F1  | Every input has a visible label or instructions, and a placeholder is never the only label                    | read each field with the screen reader and with the field filled in          | WCAG 2.2 SC 3.3.2 Labels or Instructions (A)            |
| F2  | The label is programmatically tied to the input                                                               | check `for`/`id` or wrapping `<label>`; clicking the label focuses the field | SC 1.3.1 Info and Relationships (A); SC 4.1.2 (A)       |
| F3  | An error is identified in text and names the field, not only by colour or an icon                             | submit empty and wrong; read the error without looking at colour             | SC 3.3.1 Error Identification (A); SC 1.4.1 (A)         |
| F4  | An error is announced, or focus moves to it or to a summary                                                   | submit with the screen reader running; listen                                | SC 4.1.3 Status Messages (AA); SC 3.3.1 (A)             |
| F5  | Where a correction is known, the error message suggests it                                                    | enter a date in the wrong format; read the message                           | SC 3.3.3 Error Suggestion (AA)                          |
| F6  | Legal, financial and data-changing submissions can be reviewed, corrected or reversed                         | complete a purchase or a deletion flow to the confirmation step              | SC 3.3.4 Error Prevention (Legal, Financial, Data) (AA) |
| F7  | Fields collecting the user's own data carry the right `autocomplete` token                                    | read the `autocomplete` attribute on name, email, address and payment fields | SC 1.3.5 Identify Input Purpose (AA)                    |
| F8  | Information already entered in the same process is not asked for again, unless essential                      | walk a multi-step process; note any re-entry                                 | SC 3.3.7 Redundant Entry (A)                            |
| F9  | Logging in needs no cognitive function test, or offers an alternative (paste allowed, password managers work) | try pasting into the password field; check the CAPTCHA has an alternative    | SC 3.3.8 Accessible Authentication (Minimum) (AA)       |
| F10 | A session time limit can be turned off, adjusted or extended, with warning                                    | find the timeout; wait for it                                                | SC 2.2.1 Timing Adjustable (A)                          |

## Why each one

**F4 is the one only a screen reader finds.** An error message that appears in
red next to the field is visible and silent; the user submits, hears nothing,
and does not know the form failed.

**F9** is new in WCAG 2.2, and the common failure is not a CAPTCHA — it is a
password field that blocks paste, which breaks every password manager and
forces the user to transcribe.

**F8** is also new in 2.2. A checkout that asks for the billing address again
after the shipping address, with no "same as" option, fails it.
