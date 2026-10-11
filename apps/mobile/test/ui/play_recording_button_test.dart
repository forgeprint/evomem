import 'dart:typed_data';

import 'package:evomem_mobile/src/audio/recording_controller.dart';
import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/ui/play_recording_button.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;
import 'package:http/testing.dart';

class FakePlayer implements RecordingPlayer {
  new({required this.canPlay});

  @override
  final bool canPlay;

  final List<RecordingBytes> played = [];
  int stops = 0;

  @override
  Future<void> play(RecordingBytes recording) async => played.add(recording);

  @override
  Future<void> stop() async => stops++;
}

class FakeStore implements RecordingStore {
  final Map<String, RecordingBytes> held = {};

  @override
  Future<String> newTarget() async => 'target';

  @override
  Future<int?> claim(String handle) async => held[handle]?.bytes.length;

  @override
  Future<RecordingBytes?> read(String handle) async => held[handle];

  @override
  Future<void> remove(String handle) async => held.remove(handle);

  /// What was kept from the server, by remote identifier.
  final Map<String, RecordingBytes> keptCopies = {};

  @override
  Future<void> keep(String remoteId, RecordingBytes recording) async {
    if (!canKeep(recording.mediaType)) return;
    keptCopies[remoteId] = recording;
  }

  @override
  Future<RecordingBytes?> kept(String remoteId) async => keptCopies[remoteId];
}

Note noteWith(Map<String, dynamic> metadata) {
  final now = DateTime.now();
  return Note(
    id: 'n1',
    projectId: 'default',
    content: 'Voice note, 0:03',
    sourceType: 'audio',
    createdAt: now,
    updatedAt: now,
    metadata: metadata,
  );
}

Future<void> pump(
  WidgetTester tester,
  Note note, {
  required FakePlayer player,
  required FakeStore store,
}) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        recordingPlayerProvider.overrideWithValue(player),
        recordingStoreProvider.overrideWithValue(store),
      ],
      child: MaterialApp(home: Scaffold(body: PlayRecordingButton(note))),
    ),
  );
}

/// A note the server has audio for and this device does not: pulled from
/// another device, so there is no local path and no local bytes.
Note pulledNote() => Note(
  id: 'n1',
  projectId: 'default',
  content: 'Voice note, 0:03',
  sourceType: 'audio',
  createdAt: DateTime.now(),
  updatedAt: DateTime.now(),
  metadata: const {metaAwaitingTranscription: true},
  remoteId: '01M4D3H3HNMFM69N4MHNAYBZ1X',
);

Future<void> pumpWithServer(
  WidgetTester tester,
  Note note, {
  required FakePlayer player,
  required FakeStore store,
  required MockClient client,
}) async {
  await tester.pumpWidget(
    ProviderScope(
      overrides: [
        recordingPlayerProvider.overrideWithValue(player),
        recordingStoreProvider.overrideWithValue(store),
        httpClientProvider.overrideWithValue(client),
        serverSettingsProvider.overrideWith(
          (ref) async => (url: 'http://server.test', token: 'tok'),
        ),
      ],
      child: MaterialApp(home: Scaffold(body: PlayRecordingButton(note))),
    ),
  );
  await tester.pump();
}

void main() {
  testWidgets('a note with no recording offers nothing', (tester) async {
    await pump(
      tester,
      noteWith(const {}),
      player: FakePlayer(canPlay: true),
      store: FakeStore(),
    );
    expect(find.byType(FilledButton), findsNothing);
    expect(find.textContaining('Play'), findsNothing);
  });

  // A button that would do nothing is worse than no button.
  testWidgets('a build that cannot play offers nothing', (tester) async {
    await pump(
      tester,
      noteWith(const {metaLocalPath: 'blob:x'}),
      player: FakePlayer(canPlay: false),
      store: FakeStore(),
    );
    expect(find.textContaining('Play'), findsNothing);
  });

  testWidgets('playing hands the handle to the player', (tester) async {
    final player = FakePlayer(canPlay: true);
    final store = FakeStore()
      ..held['blob:x'] = RecordingBytes(
        Uint8List.fromList([1, 2, 3]),
        'audio/webm',
      );

    await pump(
      tester,
      noteWith(const {metaLocalPath: 'blob:x'}),
      player: player,
      store: store,
    );
    await tester.tap(find.textContaining('Play'));
    await tester.pumpAndSettle();

    expect(player.played.single.bytes, [1, 2, 3]);
    expect(player.played.single.mediaType, 'audio/webm');
  });

  // The bytes do not survive a reload in a browser (ADR-0029). Pointing an
  // audio element at a dead blob fails in a way nobody can read, so it is
  // checked first and said plainly instead.
  testWidgets('a recording that is gone and unsynced says so', (tester) async {
    final player = FakePlayer(canPlay: true);

    await pump(
      tester,
      noteWith(const {metaLocalPath: 'blob:gone'}),
      player: player,
      store: FakeStore(),
    );
    await tester.tap(find.textContaining('Play'));
    await tester.pumpAndSettle();

    expect(player.played, isEmpty);
    expect(find.textContaining('nothing to play'), findsOneWidget);
  });

  // The note stands for audio the server has, even though this device no
  // longer holds the bytes. That is the case the endpoint exists for.
  testWidgets('a synced note still offers the button', (tester) async {
    await pump(
      tester,
      Note(
        id: 'n1',
        projectId: 'default',
        content: 'Voice note, 0:03',
        sourceType: 'audio',
        createdAt: DateTime.now(),
        updatedAt: DateTime.now(),
        metadata: const {metaAwaitingTranscription: true},
        remoteId: '01M4D3H3HNMFM69N4MHNAYBZ1X',
      ),
      player: FakePlayer(canPlay: true),
      store: FakeStore(),
    );
    expect(find.textContaining('Play'), findsOneWidget);
  });

  // The whole point. Audio recorded on one device and pulled onto another
  // used to be fetched again on every play, so listening to it needed the
  // network every single time — which is not what "notes and recordings
  // are kept on the device" means.
  testWidgets('a recording fetched from the server is played and kept', (
    tester,
  ) async {
    final player = FakePlayer(canPlay: true);
    final store = FakeStore();
    var requests = 0;
    final client = MockClient((request) async {
      requests++;
      expect(request.url.path, '/notes/01M4D3H3HNMFM69N4MHNAYBZ1X/audio');
      // The token is a header, never a query string (ADR-0010).
      expect(request.headers['Authorization'], 'Bearer tok');
      expect(request.url.query, isEmpty);
      return http.Response.bytes(
        [4, 5, 6],
        200,
        headers: {'content-type': 'audio/mp4'},
      );
    });

    await pumpWithServer(
      tester,
      pulledNote(),
      player: player,
      store: store,
      client: client,
    );
    await tester.tap(find.textContaining('Play'));
    await tester.pumpAndSettle();

    expect(requests, 1);
    expect(player.played.single.bytes, [4, 5, 6]);
    expect(store.keptCopies['01M4D3H3HNMFM69N4MHNAYBZ1X']?.bytes, [
      4,
      5,
      6,
    ], reason: 'it was kept, or the next play needs the network again');
  });

  // Proven by taking the server away: a second play that still works can
  // only have come from this device.
  testWidgets('playing it again needs no network at all', (tester) async {
    final player = FakePlayer(canPlay: true);
    final store = FakeStore()
      ..keptCopies['01M4D3H3HNMFM69N4MHNAYBZ1X'] = RecordingBytes(
        Uint8List.fromList([4, 5, 6]),
        'audio/mp4',
      );
    final client = MockClient((request) async {
      fail('the kept copy was not used: ${request.url}');
    });

    await pumpWithServer(
      tester,
      pulledNote(),
      player: player,
      store: store,
      client: client,
    );
    await tester.tap(find.textContaining('Play'));
    await tester.pumpAndSettle();

    expect(player.played.single.bytes, [4, 5, 6]);
    expect(player.played.single.mediaType, 'audio/mp4');
  });
}
