import 'dart:io';
import 'dart:typed_data';

import 'package:audioplayers/audioplayers.dart';
import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/audio/voice_recorder.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

/// The store a phone uses: files in the application documents directory.
RecordingStore newRecordingStore() => FileRecordingStore();

/// Nothing to add here: a phone shows its own permission dialog, and what it
/// says afterwards is between the person and the operating system.
Future<String> describeMicrophone() async => '';

/// The player a phone uses.
RecordingPlayer newRecordingPlayer() => PackageRecordingPlayer();

/// [RecordingPlayer] over the `audioplayers` package.
///
/// Verified against audioplayers 6.8.1 and audioplayers_darwin 6.5.0 /
/// audioplayers_android 5.3.0, read from the pub cache on 2026-10-10:
/// `AudioPlayer()`, `play(Source)`, `stop()`, and `BytesSource(bytes,
/// mimeType:)`.
///
///   https://pub.dev/packages/audioplayers
///
/// Bytes, not a path, because that is what the port takes and the bytes can
/// have come from the server rather than this device (ADR-0029). On Android
/// the bytes go across natively; on iOS the package writes them to a
/// temporary file and plays that, which is the workaround this would
/// otherwise have had to write by hand — so it is better left to the one
/// place that already does it.
///
/// Neither platform's floor moves: audioplayers_android asks for minSdk 19
/// against this app's 24, and audioplayers_darwin for iOS 13.0 against the
/// project's 15.0 (ADR-0031).
class PackageRecordingPlayer implements RecordingPlayer {
  final AudioPlayer _player = AudioPlayer();

  File? _scratch;

  @override
  bool get canPlay => true;

  @override
  Future<void> play(RecordingBytes recording) async {
    // Asked before anything is handed to the platform. Giving WebM to
    // AVFoundation produces an error that names a status code and nothing
    // a person can act on; this names what happened and where it does
    // work.
    final refusal = playbackRefusal(
      recording.mediaType,
      onApplePlatform: Platform.isIOS || Platform.isMacOS,
    );
    if (refusal != null) throw PlaybackFailure(refusal);

    await stop();

    // A file this writes, with an extension this chooses, rather than
    // `BytesSource`.
    //
    // Measured on an iOS 27 simulator on 2026-10-10: a valid AAC-LC m4a
    // fetched back from the server, handed over as bytes, failed with
    // CoreMedia err=-12864. audioplayers' own iOS path writes the bytes to
    // a temporary file named by a hash of them — no extension — and tells
    // AVFoundation the type through `AVURLAssetOverrideMIMETypeKey`, which
    // did not save it. An extension AVFoundation recognises does.
    final file = File(
      p.join(
        (await getTemporaryDirectory()).path,
        'evomem-playback${playbackExtension(recording.mediaType)}',
      ),
    );
    await file.writeAsBytes(recording.bytes, flush: true);
    _scratch = file;

    await _player.play(DeviceFileSource(file.path));
  }

  @override
  Future<void> stop() async {
    await _player.stop();
    // The copy goes when playing stops. It is a second copy of somebody's
    // own audio and there is no reason for it to outlive the listening.
    final scratch = _scratch;
    _scratch = null;
    if (scratch != null && scratch.existsSync()) {
      try {
        await scratch.delete();
      } on Object {
        // A file that will not delete is not worth failing playback over;
        // the next play overwrites it under the same name anyway.
      }
    }
  }
}

/// [RecordingStore] over the filesystem.
class FileRecordingStore implements RecordingStore {
  /// Creates a store. [directory] is for a test that does not want
  /// `path_provider`, which needs a platform to answer.
  new({this.directory});

  /// Where recordings go, when a test said so. Null means ask the platform,
  /// which is what every build does and no test can.
  final Future<Directory>? directory;

  Future<Directory> _recordings() async {
    if (directory != null) return await directory!;
    final documents = await getApplicationDocumentsDirectory();
    return Directory(p.join(documents.path, 'recordings'));
  }

  @override
  Future<String> newTarget() async {
    final directory = await _recordings();
    await directory.create(recursive: true);
    // Named by when it was made, which is unique enough on one device and
    // readable in a file listing. The note's own identifier is not
    // available yet: it is minted when the recording stops.
    return p.join(
      directory.path,
      'note-${DateTime.now().millisecondsSinceEpoch}$recordingExtension',
    );
  }

  @override
  Future<int?> claim(String handle) async {
    final file = File(handle);
    if (!file.existsSync()) return null;
    return await file.length();
  }

  @override
  Future<RecordingBytes?> read(String handle) async {
    final file = File(handle);
    if (!file.existsSync()) return null;
    // The encoder wrote m4a, which is what this app asked it for.
    return RecordingBytes(
      Uint8List.fromList(await file.readAsBytes()),
      recordingMediaType,
    );
  }

  @override
  Future<void> remove(String handle) async {
    final file = File(handle);
    if (file.existsSync()) await file.delete();
  }

  @override
  Future<void> keep(String remoteId, RecordingBytes recording) async {
    final name = _keptName(remoteId);
    if (name == null || !canKeep(recording.mediaType)) return;

    final directory = await _recordings();
    await directory.create(recursive: true);
    final file = File(
      p.join(directory.path, '$name${playbackExtension(recording.mediaType)}'),
    );
    try {
      await file.writeAsBytes(recording.bytes, flush: true);
    } on Object {
      // A full disk is not a reason to fail the playback that is already
      // holding the bytes. The next play asks the server again.
      if (file.existsSync()) {
        try {
          await file.delete();
        } on Object {
          // Nothing left to try; a half-written file is found by its
          // extension next time and would play as truncated audio, so
          // deleting it is the attempt worth making and not worth
          // failing over.
        }
      }
    }
  }

  @override
  Future<RecordingBytes?> kept(String remoteId) async {
    final name = _keptName(remoteId);
    if (name == null) return null;

    final directory = await _recordings();
    if (!directory.existsSync()) return null;

    for (final extension in keptExtensions) {
      final file = File(p.join(directory.path, '$name$extension'));
      if (!file.existsSync()) continue;
      final mediaType = keptMediaType(extension);
      if (mediaType == null) continue;
      return RecordingBytes(
        Uint8List.fromList(await file.readAsBytes()),
        mediaType,
      );
    }
    return null;
  }

  /// The filename stem for [remoteId], or null when it will not be used as
  /// one.
  ///
  /// The identifier comes from the server, so it is never pasted into a
  /// path as it stands: `../../x` or a separator would write outside the
  /// recordings directory. A ULID is Crockford base32, so the characters
  /// allowed here cover every identifier this system mints and reject
  /// everything that could traverse.
  String? _keptName(String remoteId) {
    if (remoteId.isEmpty || remoteId.length > 64) return null;
    if (!RegExp(r'^[A-Za-z0-9_-]+$').hasMatch(remoteId)) return null;
    return 'kept-$remoteId';
  }
}
