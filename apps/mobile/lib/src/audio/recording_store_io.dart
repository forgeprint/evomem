import 'dart:io';
import 'dart:typed_data';

import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/audio/voice_recorder.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

/// The store a phone uses: files in the application documents directory.
RecordingStore newRecordingStore() => FileRecordingStore();

/// Nothing to add here: a phone shows its own permission dialog, and what it
/// says afterwards is between the person and the operating system.
Future<String> describeMicrophone() async => '';

/// There is no player on this side yet.
RecordingPlayer newRecordingPlayer() => const NoRecordingPlayer();

/// A player that says it cannot play, so nothing offers the button.
///
/// Playing a file on a phone needs a package this app does not have, and
/// the phone cannot currently be built at all — there are no android/ios
/// build files. That is when a player package earns its place, not before
/// (ADR-0029).
class NoRecordingPlayer implements RecordingPlayer {
  /// Creates the player.
  const new();

  @override
  bool get canPlay => false;

  @override
  Future<void> play(RecordingBytes recording) async {}

  @override
  Future<void> stop() async {}
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
}
