# Screen reader

Run each essential process with a real screen reader: NVDA with a Chromium
browser or Firefox on Windows, VoiceOver with Safari on macOS or iOS. Record the
screen reader, its version and the browser in every log row, because results
differ between pairs.

| #   | Check                                                                                               | How                                                                                          | Source                                                |
| --- | --------------------------------------------------------------------------------------------------- | -------------------------------------------------------------------------------------------- | ----------------------------------------------------- |
| SR1 | Every informative image has a text alternative that serves its purpose; decorative ones are ignored | walk the images with the screen reader; read the `alt` against what the image is for         | WCAG 2.2 SC 1.1.1 Non-text Content (A)                |
| SR2 | Headings, lists, tables and landmarks are real structure, not styling                               | open the screen reader's headings and landmarks lists; compare with what the page looks like | SC 1.3.1 Info and Relationships (A)                   |
| SR3 | Every control announces a name, a role and its state, and state changes are announced               | Tab to each control; listen for all three; toggle it and listen again                        | SC 4.1.2 Name, Role, Value (A)                        |
| SR4 | A native element is used wherever one exists; ARIA only where HTML has no equivalent                | grep for `role="button"`, `role="link"`, `role="checkbox"` on non-native elements            | ARIA in HTML; APG — "No ARIA is better than bad ARIA" |
| SR5 | The accessible name contains the visible label text                                                 | compare each control's visible text with its `aria-label` or `aria-labelledby`               | SC 2.5.3 Label in Name (A)                            |
| SR6 | Status messages (saved, results found, item added) are announced without moving focus               | trigger each one and listen; find the live region or `role="status"`                         | SC 4.1.3 Status Messages (AA)                         |
| SR7 | The page has a descriptive title and a correct `lang`; passages in another language are marked      | read `<title>` and `<html lang>`; listen for pronunciation changes                           | SC 2.4.2 Page Titled (A); 3.1.1 (A); 3.1.2 (AA)       |
| SR8 | Link text makes sense in context; no bare "click here" lists                                        | open the screen reader's links list                                                          | SC 2.4.4 Link Purpose (In Context) (A)                |
| SR9 | Headings and labels describe their topic or purpose                                                 | read the headings list alone; can you tell what each section is                              | SC 2.4.6 Headings and Labels (AA)                     |

## Why each one

**SR3 is where custom components fail.** A toggle that changes colour when
pressed and announces nothing has a state only sighted users can see. Listen
for the state before and after, not only the name.

**SR4** is a refusal as much as a check. Every ARIA role on a non-native element
is a promise to build focus, keyboard handling and state by hand; a native
element keeps that promise for free. Most component findings end here.

**SR6** is the one people cannot find without a screen reader running: a
"saved" toast that appears and fades is invisible to a user who never had focus
on it.
