import 'dart:io';
import 'dart:typed_data';

import 'package:evomem_mobile/src/audio/recording_store_io.dart';
import 'package:evomem_mobile/src/audio/voice_recorder.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late Directory directory;
  late FileRecordingStore store;

  setUp(() {
    directory = Directory.systemTemp.createTempSync('evomem-recordings');
    store = FileRecordingStore(directory: Future.value(directory));
  });

  tearDown(() {
    if (directory.existsSync()) directory.deleteSync(recursive: true);
  });

  test('a target is inside the directory and named for the encoder', () async {
    final target = await store.newTarget();
    expect(target, startsWith(directory.path));
    expect(target, endsWith(recordingExtension));
  });

  test(
    'claim reports the size, and nothing for a file that is not there',
    () async {
      final target = await store.newTarget();
      expect(await store.claim(target), isNull);

      File(target).writeAsBytesSync(Uint8List.fromList([1, 2, 3]));
      expect(await store.claim(target), 3);
    },
  );

  test('read gives the bytes and says what they are', () async {
    final target = await store.newTarget();
    File(target).writeAsBytesSync(Uint8List.fromList([7, 7]));

    final recording = await store.read(target);
    expect(recording, isNotNull);
    expect(recording!.bytes, [7, 7]);
    // The encoder wrote m4a; a browser would say something else, which is
    // the whole reason the type travels with the bytes (ADR-0029).
    expect(recording.mediaType, recordingMediaType);
  });

  test('read gives nothing once it is removed', () async {
    final target = await store.newTarget();
    File(target).writeAsBytesSync(Uint8List.fromList([1]));

    await store.remove(target);
    expect(await store.read(target), isNull);
    // Removing one that is already gone is not an error: a cancel that
    // races a failed start would otherwise throw.
    await store.remove(target);
  });
}
