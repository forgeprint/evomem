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

/// Where this build keeps recordings. A test overrides it.
final recordingStoreProvider = Provider<RecordingStore>(
  (ref) => platform.newRecordingStore(),
);
