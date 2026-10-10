# ADR-0031: the app on a phone

**Status:** accepted · **Date:** 2026-10-10 · **Amends:** ADR-0029

## Context

The Flutter app had been built for exactly one target: the browser. There
were no Android or iOS build files at all — no `settings.gradle`, no
`build.gradle`, no Xcode project. Two files had been written by hand against
that absence: `AndroidManifest.xml`, carrying `RECORD_AUDIO` and a comment
saying the minimum SDK would have to be set somewhere once there was
somewhere to set it, and `Info.plist`, carrying `NSMicrophoneUsageDescription`
and the same note about a deployment target.

Three separate pieces of work were waiting on this one absence: recording on
a real microphone, playing a recording back on a phone (`RecordingPlayer`'s
io implementation says it cannot play, and says why), and writing down the
platform floors the `record` package needs.

## Decision

### 1. The platform files are generated, then corrected and pinned

`flutter create --platforms=android,ios` writes them. Its defaults are not
kept:

| what it generated | what is in the tree | why |
|---|---|---|
| `com.evomem.evomem_mobile` | `com.evomem.mobile` | the iOS bundle id was already that; two identities for one app is a shipping accident |
| `minSdk = flutter.minSdkVersion` | `minSdk = 24` | the floor is the plugins', not Flutter's |
| `package=` in the manifest | removed | the Android Gradle Plugin refuses it |
| a stock `widget_test.dart` | deleted | it tests a counter app that does not exist here |

The floors were read from the pub cache on 2026-10-10, not recalled:
`record_android` 2.2.0 and `flutter_secure_storage` 11.2.0 both declare
`minSdk = 24`; `record_ios` 2.1.1 declares iOS 12.0 in its podspec and its
`Package.swift`. The Xcode project's `IPHONEOS_DEPLOYMENT_TARGET` is 15.0,
above that floor, so nothing is lowered.

`flutter create` hands its defaults back every time anybody runs it again,
and none of those defaults fails a Dart build. `test/platform/build_files_test.dart`
reads the files and fails when they drift.

### 2. A phone's launch needs the UIScene manifest

iOS 27 requires it. `flutter create` writes a `SceneDelegate` and the Xcode
project but leaves an existing `Info.plist` untouched — and this one predated
the Xcode project, so it had no `UIApplicationSceneManifest`. The build was
clean. The app trapped at launch with `EXC_BREAKPOINT` before any Dart ran,
and the only thing that said why was `flutter run`, which printed the
migration notice. The manifest block is copied from the template Flutter
3.47.5 ships.

### 3. A recorder that hangs is a failure, after ten seconds

Running it on a simulator whose audio input CoreAudio refuses showed
something the browser never did: `record`'s `start` **never returns and never
throws**. The button stayed on "Record", the logbook stayed empty, and the
only trace was a line in the system log. That is the same shape of failure
the logbook was built for in ADR-0029 — a recording that does not happen and
does not say so — reached by a different road.

So `start()` and `isRecording()` are bounded by `recorderPatience`, ten
seconds. Asking for permission is **not** bounded: that call waits for a
person to read a dialog. On the timeout the recorder is cancelled on a best
effort, because a half-open recorder makes the next attempt fail too.

Separately, `_store.newTarget()` was outside every `try`. An exception from
it left `start()` without touching the state, the log, or the message the
screen shows — the same dead button by a third road. It is inside now.

### 4. Every screen gets a way back

Screens are navigated to with `context.go`, which replaces rather than
stacks, so Flutter adds no back button. In a browser the browser's own back
button hid this completely. On a phone the Log screen was a room with no
door, and so were Groups, Sources, Devices and a group's detail. `BackTo`
names where back is, since there is no stack to pop.

## Costs

- **Android is unverified here.** This machine has no JDK and no Android SDK,
  so the Gradle files have never been run. They are written against what the
  plugins declare and pinned by a test, which is not the same as a build. The
  first person with an Android SDK will find out.
- **Recording on a phone is still unproven.** The simulator's audio input is
  refused by CoreAudio, so what was demonstrated is that the failure is now
  reported, not that a recording works. A real device, or a simulator with a
  working input, is the only way to close that.
- **Playback on a phone is still not written.** The blocker is gone; the
  player package is not chosen. `NoRecordingPlayer` still says it cannot
  play, and its comment now points here.
- **Ten seconds is a guess.** It is long enough for a slow audio session and
  short enough that somebody notices the button answered. Nothing measured
  it, because the only measurement available was a recorder that never
  answered at all.
- **`NSAllowsArbitraryLoads` is still set.** It predates this record and lets
  the app talk to a plain-HTTP server on a LAN, which is how it is used. It
  is broader than the `localhost` exception beside it, and it is not revisited
  here.

## Amendment, 2026-10-10: playback on a phone

Cost 3 above said the player package was not chosen. It is now:
**audioplayers 6.8.1**, pinned exactly like everything else.

### Why this one

The port takes bytes, not a path, because the bytes can have come from the
server rather than this device (ADR-0029). audioplayers covers Android, iOS
and the rest with a small surface — `AudioPlayer`, `play(Source)`, `stop()` —
and neither of its platform floors moves anything: audioplayers_android 5.3.0
asks for minSdk 19 against this app's 24, and audioplayers_darwin 6.5.0 for
iOS 13.0 against the project's 15.0. Read from the pub cache, 2026-10-10.

The browser keeps its own player. `BlobRecordingPlayer` already works, is
documented, and revokes its object URLs; replacing something that works with
something new buys nothing. The cost is that audioplayers_web rides along in
the web build without being used.

### A file this app names, not bytes handed over

`BytesSource` looked like the obvious fit and does not work here. Measured on
an iOS 27 simulator: a valid AAC-LC m4a, fetched back from the server with
`Content-Type: audio/mp4`, failed with CoreMedia `err=-12864`. The package's
iOS path writes the bytes to a temporary file named by a **hash of them**,
with no extension, and tells AVFoundation the type through
`AVURLAssetOverrideMIMETypeKey`, which did not rescue it.

So this writes the file itself, with an extension `playbackExtension` chooses
from the media type, and plays a `DeviceFileSource`. The copy is deleted when
playing stops: it is a second copy of somebody's own audio and has no reason
to outlive the listening.

### What a phone cannot play, and says so

A phone records AAC-LC in m4a; a browser records Opus in WebM. AVFoundation
has no WebM demuxer — Safari plays WebM through WebKit's own, which native
code cannot reach (Apple Developer Forums, read 2026-10-10). So a note
recorded in a browser, synced, and opened on an iPhone is unplayable there,
and the platform's error for it says nothing a person can use.

`playbackRefusal` is asked first and the sentence names where it does work:
*"It still plays in a browser."* This is not a bug being papered over; it is
the cost ADR-0029 took on when it chose Opus for the browser, becoming
visible now that a second device can ask for the same recording.

### Verified, end to end

On an iOS 27 simulator against the Docker server, 2026-10-10:

- An m4a recording fetched from `GET /notes/{id}/audio`: the player reached
  `rate 1.000` and ran to `StopAtEnd` — 3.4 seconds, the length of the clip.
- The same app, on a note whose recording was made in a browser: the refusal,
  in words, on screen.

### What this still does not settle

- **Android playback is unrun**, like the rest of the Android build.
- **A browser recording stays unplayable on a phone.** Transcoding — on
  upload, or on demand — would fix it and is not decided here. Recording AAC
  in the browser is the other direction, and ADR-0029 already measured why
  that does not work.
- **Nothing shows playback position**, and there is no scrubbing. One button,
  play and stop.
