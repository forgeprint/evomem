# Keyboard and focus

Put the mouse away and complete every essential process with Tab, Shift+Tab,
Enter, Space, Escape and the arrow keys. Most of what fails here is invisible to
a scanner and obvious within a minute of trying.

| #   | Check                                                                                                        | How                                                                                             | Source                                      |
| --- | ------------------------------------------------------------------------------------------------------------ | ----------------------------------------------------------------------------------------------- | ------------------------------------------- |
| K1  | Every function available by pointer is available by keyboard                                                 | complete each essential process in `sample.md` without a pointer; log each step                 | WCAG 2.2 SC 2.1.1 Keyboard (A)              |
| K2  | Focus never gets stuck; every dialog, menu and embedded widget can be left                                   | open each one and try to Tab and Escape out                                                     | SC 2.1.2 No Keyboard Trap (A)               |
| K3  | The focus order follows the meaning of the page; no positive `tabindex`                                      | Tab through and write down the order; grep for `tabindex="[1-9]`                                | SC 2.4.3 Focus Order (A)                    |
| K4  | Every focused element has a visible indicator                                                                | Tab through; grep the styles for `outline: none` or `outline: 0` with no replacement            | SC 2.4.7 Focus Visible (AA)                 |
| K5  | A focused element is never entirely hidden by a sticky header, footer, banner or cookie notice               | Tab through with every sticky element present; check the element is at least partly visible     | SC 2.4.11 Focus Not Obscured (Minimum) (AA) |
| K6  | A skip link or landmarks let a keyboard user bypass repeated navigation                                      | press Tab once on load; check the skip link works, or list the landmarks with the screen reader | SC 2.4.1 Bypass Blocks (A)                  |
| K7  | Receiving focus does not change the context — no navigation, submit or new window on focus alone             | Tab onto each control and wait                                                                  | SC 3.2.1 On Focus (A)                       |
| K8  | A dialog moves focus into itself on open and returns it to the trigger on close                              | open and close each dialog; note where focus lands both times                                   | APG — Dialog (Modal) pattern                |
| K9  | Each custom widget answers the keys its APG pattern names (tabs: arrows; menu: arrows and Escape; and so on) | for each custom widget, compare its key handling with the pattern's keyboard interaction table  | APG — the widget's pattern; SC 2.1.1        |
| K10 | Single-character shortcuts can be turned off or remapped, or only work when their control has focus          | find the key handlers bound without a modifier                                                  | SC 2.1.4 Character Key Shortcuts (A)        |

## Why each one

**K1 is the check the rest depend on.** A control built from a `<div>` with a
click handler cannot be reached, and nothing else in this file matters for a
user who cannot get to it. It is also the finding a native `<button>` fixes in
one line.

**K5** is new in WCAG 2.2 and a sticky layout fails it without anybody
noticing: a sticky
header plus a page that scrolls focus to the top edge means the focused link is
underneath the header. `scroll-padding` on the scroll container is a sufficient
technique named in the Understanding document.

**K8** is not itself a success criterion; it is how the dialog pattern meets
2.4.3 and 2.1.2 in practice. Cite the SC in the finding and the pattern in the
fix.
