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
