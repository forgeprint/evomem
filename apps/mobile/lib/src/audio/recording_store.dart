import 'dart:typed_data';

import 'package:evomem_mobile/src/audio/recording_store_io.dart'
    if (dart.library.js_interop) 'package:evomem_mobile/src/audio/recording_store_web.dart'
    as platform;
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// A finished recording: its bytes and what they are.
///
/// The media type travels with the bytes because it is not a constant. A
/// phone writes what its encoder produced; a browser writes whatever its
/// MediaRecorder agreed to, which is usually not the same thing (ADR-0029).
class RecordingBytes {
  /// Creates a recording.
  const new(this.bytes, this.mediaType);

  /// What was recorded.
  final Uint8List bytes;

  /// What the bytes are, for the upload to declare.
  final String mediaType;
}

/// Where a recording lives between being made and being uploaded.
///
/// A port, like `NotesStore` in ADR-0022 and for the same reason: the thing
/// that differs between a phone and a browser is where bytes live, and that
/// is one interface rather than a `kIsWeb` branch in four places.
abstract interface class RecordingStore {
  /// Where to record. On a phone this is a path; in a browser the recorder
  /// ignores it and hands back its own handle when it stops (ADR-0029).
  Future<String> newTarget();

  /// Takes ownership of what the recorder returned from `stop()`, and
  /// reports how many bytes it holds.
  ///
  /// Null when there is nothing there. In a browser this is where the bytes
  /// are read out of the blob, so that the URL's own lifetime stops
  /// mattering for the rest of the session.
  Future<int?> claim(String handle);

  /// The bytes of a recording, or null when they are gone.
  Future<RecordingBytes?> read(String handle);

  /// Forgets a recording.
  Future<void> remove(String handle);
}

/// Plays back a recording this device still holds.
///
/// A port like [RecordingStore], and for the same reason: a browser has an
/// audio element and a phone needs a package that is not here yet. It
/// reports whether it can play at all, so a button that would do nothing is
/// never offered (ADR-0029).
abstract interface class RecordingPlayer {
  /// Whether this build can play anything.
  bool get canPlay;

  /// Plays [recording] from the start.
  ///
  /// Bytes rather than a handle, because the bytes can come from two
  /// places: this device, or the server once the recording has been
  /// uploaded. Taking a URL instead would mean putting the API token in a
  /// query string, since an audio element sends no headers (ADR-0029).
  Future<void> play(RecordingBytes recording);

  /// Stops whatever is playing.
  Future<void> stop();
}

/// A recording this device will not play, with the reason in words.
///
/// `toString` is the message, not the class name: a release build has no
/// class names, and the one thing that reached a person the last time this
/// was got wrong was `Instance of 'minified:fn'`.
class PlaybackFailure implements Exception {
  /// Creates a failure that says [message].
  const new(this.message);

  /// What to tell somebody, in a sentence.
  final String message;

  @override
  String toString() => message;
}

/// Why [mediaType] cannot be played here, or null when it can be.
///
/// A phone records AAC-LC in an `.m4a` and a browser records Opus in a WebM
/// container (ADR-0029), and the two do not meet: AVFoundation has no WebM
/// demuxer — Safari plays WebM through WebKit's own, which native code
/// cannot reach (Apple Developer Forums, read 2026-10-10). So a note
/// recorded in a browser and fetched back from the server is unplayable on
/// an iPhone, and the platform's own error for it says nothing useful.
///
/// Pure, and taking the platform as an argument, so the rule can be tested
/// without being on that platform.
String? playbackRefusal(String mediaType, {required bool onApplePlatform}) {
  if (onApplePlatform && mediaType.split(';').first.trim() == 'audio/webm') {
    return 'This recording was made in a browser, which writes Opus in a '
        'WebM container. Apple platforms cannot read WebM, so it will not '
        'play here. It still plays in a browser.';
  }
  return null;
}

/// The filename extension to give [mediaType] when writing it out to play.
///
/// AVFoundation decides what a local file is from its extension, and gets it
/// wrong — or refuses — without one: a valid AAC-LC m4a handed over with no
/// extension failed with CoreMedia err=-12864 on an iOS 27 simulator
/// (2026-10-10). So the extension is chosen here rather than left to chance.
///
/// The types are the ones this system actually produces and stores: `shared/audio`
/// maps what an upload may declare, a phone records m4a and a browser webm
/// (ADR-0029, ADR-0016). Anything else falls back to the phone's own format,
/// because a wrong extension is no worse than none and the common case is a
/// recording this app made.
String playbackExtension(String mediaType) =>
    switch (mediaType.split(';').first.trim().toLowerCase()) {
      'audio/mp4' || 'audio/m4a' || 'audio/x-m4a' => '.m4a',
      'audio/aac' => '.aac',
      'audio/mpeg' || 'audio/mp3' => '.mp3',
      'audio/wav' || 'audio/x-wav' || 'audio/wave' => '.wav',
      'audio/ogg' || 'audio/opus' => '.ogg',
      'audio/webm' => '.webm',
      _ => '.m4a',
    };

/// How this build plays a recording back. A test overrides it.
final recordingPlayerProvider = Provider<RecordingPlayer>(
  (ref) => platform.newRecordingPlayer(),
);

/// Why the microphone is not available, in words somebody can act on.
///
/// Empty when there is nothing useful to add — on a phone the platform's own
/// dialog is the whole story. In a browser there are three different reasons
/// a refusal happens and they need different fixes, so this asks which.
///
/// A log line that says "the microphone was refused" and nothing else is the
/// half-answer this whole logbook exists to stop.
Future<String> describeMicrophone() => platform.describeMicrophone();

/// Where this build keeps recordings. A test overrides it.
final recordingStoreProvider = Provider<RecordingStore>(
  (ref) => platform.newRecordingStore(),
);
