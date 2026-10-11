import 'dart:io';
import 'dart:typed_data';

import 'package:evomem_mobile/src/audio/recording_store.dart';
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

  // A note that came from another device has no local recording: the audio
  // is on the server and used to be fetched on every single play, so
  // listening to it needed the network every time. Kept under the remote
  // identifier rather than written into the note, because the note's
  // metadata is pushed to the server as it stands.
  test('a copy kept from the server is read back as itself', () async {
    const remoteId = '01M4D3H3HNMFM69N4MHNAYBZ1X';
    expect(await store.kept(remoteId), isNull);

    await store.keep(
      remoteId,
      RecordingBytes(Uint8List.fromList([9, 8, 7]), 'audio/mp4'),
    );

    final kept = await store.kept(remoteId);
    expect(kept, isNotNull);
    expect(kept!.bytes, [9, 8, 7]);
    expect(kept.mediaType, 'audio/mp4');
  });

  // The type has to survive, not be assumed. A recording made in a browser
  // is Opus in WebM, and AVFoundation cannot read it (ADR-0029): read back
  // as m4a it would reach the player as audio it cannot play, with
  // playbackRefusal silent because the type said otherwise.
  test('a kept browser recording is still WebM afterwards', () async {
    const remoteId = 'ABCDEF';
    await store.keep(
      remoteId,
      RecordingBytes(Uint8List.fromList([1]), 'audio/webm;codecs=opus'),
    );

    expect((await store.kept(remoteId))!.mediaType, 'audio/webm');
  });

  // playbackExtension guesses .m4a for a type it does not know, so keeping
  // one would hand it back labelled audio/mp4 — bytes claiming to be
  // something they are not.
  test('a type that cannot be read back as itself is not kept', () async {
    const remoteId = 'GHIJKL';
    expect(canKeep('application/octet-stream'), isFalse);

    await store.keep(
      remoteId,
      RecordingBytes(Uint8List.fromList([1]), 'application/octet-stream'),
    );

    expect(await store.kept(remoteId), isNull);
    expect(directory.listSync(), isEmpty);
  });

  // The identifier comes from the server, so it never reaches a path as it
  // stands.
  test('an identifier that would leave the directory is refused', () async {
    for (final hostile in ['../../escaped', 'a/b', r'c\d', '', 'x' * 65]) {
      await store.keep(
        hostile,
        RecordingBytes(Uint8List.fromList([1]), 'audio/mp4'),
      );
      expect(await store.kept(hostile), isNull, reason: hostile);
    }
    expect(
      directory.listSync(recursive: true),
      isEmpty,
      reason: 'nothing was written anywhere',
    );
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
