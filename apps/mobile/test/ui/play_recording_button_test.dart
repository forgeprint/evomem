import 'dart:typed_data';

import 'package:evomem_mobile/src/audio/recording_controller.dart';
import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/ui/play_recording_button.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

class FakePlayer implements RecordingPlayer {
  new({required this.canPlay});

  @override
  final bool canPlay;

  final List<String> played = [];
  int stops = 0;

  @override
  Future<void> play(String handle) async => played.add(handle);

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

    expect(player.played, ['blob:x']);
  });

  // The bytes do not survive a reload in a browser (ADR-0029). Pointing an
  // audio element at a dead blob fails in a way nobody can read, so it is
  // checked first and said plainly instead.
  testWidgets('a recording that is gone says so and plays nothing', (
    tester,
  ) async {
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
    expect(find.textContaining('gone'), findsOneWidget);
  });
}
