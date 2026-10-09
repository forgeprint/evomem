# Zoom, contrast and targets

The criteria here are numbers — ratios, pixels, percentages — so each check
ends with a measurement written in the log, not an impression.

| #   | Check                                                                                            | How                                                                                                        | Source                                    |
| --- | ------------------------------------------------------------------------------------------------ | ---------------------------------------------------------------------------------------------------------- | ----------------------------------------- |
| Z1  | Text reaches 4.5:1 against its background, or 3:1 for large text                                 | measure each distinct pair with a contrast checker; include text over images, and placeholder text         | WCAG 2.2 SC 1.4.3 Contrast (Minimum) (AA) |
| Z2  | Focus indicators, input borders, icons and chart elements reach 3:1 against adjacent colours     | measure the indicator against the background and against the unfocused state                               | SC 1.4.11 Non-text Contrast (AA)          |
| Z3  | Colour is never the only way information is conveyed                                             | view the page in greyscale; errors, links in text and chart series must still be distinguishable           | SC 1.4.1 Use of Color (A)                 |
| Z4  | Text resizes to 200% without loss of content or function                                         | browser zoom to 200%; nothing is cut off, overlapped or unreachable; grep for `user-scalable=no`           | SC 1.4.4 Resize Text (AA)                 |
| Z5  | At 320 CSS px wide (400% zoom at 1280 px), content reflows with no horizontal scrolling for text | set the window to 1280 px and zoom to 400%, or set the viewport to 320 px; read every line                 | SC 1.4.10 Reflow (AA)                     |
| Z6  | Increased line height, paragraph, letter and word spacing lose no content                        | apply the four spacing values from the Understanding document with a bookmarklet or stylesheet             | SC 1.4.12 Text Spacing (AA)               |
| Z7  | Pointer targets are at least 24 by 24 CSS px, or meet the spacing exception                      | measure small targets (close buttons, icon buttons, pagination); draw the 24 px circle for undersized ones | SC 2.5.8 Target Size (Minimum) (AA)       |
| Z8  | Anything done by dragging can also be done with a single pointer without dragging                | find sliders, sortable lists and map panning; look for an alternative                                      | SC 2.5.7 Dragging Movements (AA)          |
| Z9  | Content shown on hover or focus can be dismissed, hovered and stays until the user moves on      | trigger each tooltip and popover; press Escape; move the pointer onto it                                   | SC 1.4.13 Content on Hover or Focus (AA)  |
| Z10 | Content is not locked to one orientation unless essential                                        | rotate a device or the emulator                                                                            | SC 1.3.4 Orientation (AA)                 |

## Why each one

**Z2 is the contrast check people skip.** Text contrast is familiar and axe
measures most of it; the focus ring, the input border and the icon-only button
are not text, need 3:1, and are exactly what a restrained palette makes too
faint.

**Z5** finds what responsive design was supposed to prevent: a data table, a
fixed-width modal or a code block that forces a low-vision user to scroll in
two directions to read one line. Content that needs two dimensions, such as a
data table, is excepted; the navigation around it is not.

**Z7** is new in WCAG 2.2 and has five exceptions — spacing, equivalent,
inline, user agent control and essential. Check the spacing exception before
reporting; a 20 px icon with enough clear space around it passes.
