# Motion and media

Movement, flashing and time-based media each have a small number of criteria
and a large effect on the users they affect. Run this pass with the operating
system's reduced-motion setting both off and on.

| #   | Check                                                                                                            | How                                                                                               | Source                                                         |
| --- | ---------------------------------------------------------------------------------------------------------------- | ------------------------------------------------------------------------------------------------- | -------------------------------------------------------------- |
| M1  | Anything that moves, blinks or scrolls automatically for more than five seconds can be paused, stopped or hidden | find carousels, marquees, animated backgrounds and auto-updating feeds; look for the control      | WCAG 2.2 SC 2.2.2 Pause, Stop, Hide (A)                        |
| M2  | Nothing flashes more than three times in any one-second period, or it stays below the thresholds                 | review videos and animations; measure where flashing is present                                   | SC 2.3.1 Three Flashes or Below Threshold (A)                  |
| M3  | Motion triggered by interaction (parallax, zoom-on-scroll) respects the reduced-motion preference                | turn on reduced motion; grep for `prefers-reduced-motion` in the styles and scripts               | SC 2.3.3 Animation from Interactions (AAA) — Observation at AA |
| M4  | Pre-recorded video with audio has accurate captions                                                              | watch a sample with captions on; auto-generated captions are checked, not assumed                 | SC 1.2.2 Captions (Prerecorded) (A)                            |
| M5  | Pre-recorded video has audio description where the visuals carry information the audio does not                  | watch with the screen off; note what is lost                                                      | SC 1.2.5 Audio Description (Prerecorded) (AA)                  |
| M6  | Pre-recorded audio-only content has a transcript                                                                 | find each podcast or audio clip; look for the transcript                                          | SC 1.2.1 Audio-only and Video-only (Prerecorded) (A)           |
| M7  | Live video with audio has captions                                                                               | if the product streams live, check the caption path                                               | SC 1.2.4 Captions (Live) (AA)                                  |
| M8  | Audio that plays automatically for more than three seconds can be paused or its volume controlled independently  | load the page with sound on                                                                       | SC 1.4.2 Audio Control (A)                                     |
| M9  | Media player controls are keyboard operable and named                                                            | Tab to the player; operate play, pause, captions and volume by keyboard and listen to their names | SC 2.1.1 (A); SC 4.1.2 (A)                                     |

## Why each one

**M2 is the only item in this expert that can cause physical harm.** Flashing
content can trigger seizures; it is Level A and it is checked even when nothing
else in this file applies.

**M3 is reported honestly.** Animation from interactions is Level AAA, so under
an AA target a failure goes under Observations, not findings. It is on the list
because the reduced-motion preference is cheap to respect and the users who set
it set it for a reason.

**M4** catches the most common media failure: auto-generated captions shipped
unreviewed. Inaccurate captions do not meet the criterion, and the only way to
know is to watch.
