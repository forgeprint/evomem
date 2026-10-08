import 'dart:io';

import 'package:evomem_mobile/src/audio/recording_controller.dart';
import 'package:evomem_mobile/src/audio/voice_recorder.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import '../sqflite_test_setup.dart' as sqflite_setup;

/// A recorder with no microphone behind it.
///
/// It writes whatever [contents] says when asked to record, which is how the
/// tests exercise an empty recording, a missing file and a normal one without
/// any hardware.
class FakeRecorder implements VoiceRecorder {
  new({
    this.permitted = true,
    this.contents = 'aac-pretend',
    this.writesFile = true,
    this.failOnStart = false,
    this.failOnStop = false,
  });

  final bool permitted;
  final String contents;
  final bool writesFile;
  final bool failOnStart;
  final bool failOnStop;

  String? startedAt;
  int cancels = 0;
  int disposes = 0;

  @override
  Future<bool> hasPermission() async => permitted;

  @override
  Future<void> start(String path) async {
    if (failOnStart) throw const FileSystemException('no microphone');
    startedAt = path;
    if (writesFile) File(path).writeAsStringSync(contents);
  }

  @override
  Future<String?> stop() async {
    if (failOnStop) throw const FileSystemException('recorder died');
    return startedAt;
  }

  @override
  Future<void> cancel() async {
    cancels++;
  }

  @override
  Future<void> dispose() async {
    disposes++;
  }
}

void main() {
  sqflite_setup.setupSqfliteFfi();

  late Directory recordings;

  setUp(() async {
    recordings = Directory.systemTemp.createTempSync('evomem-recordings');
    final db = await DatabaseHelper.instance.database;
    await db.delete('notes');
  });

  tearDown(() {
    if (recordings.existsSync()) recordings.deleteSync(recursive: true);
  });

  ProviderContainer containerWith(FakeRecorder recorder) {
    final container = ProviderContainer(
      overrides: [
        voiceRecorderProvider.overrideWithValue(recorder),
        // path_provider has no platform to answer here, so the directory is
        // supplied rather than looked up.
        recordingsDirectoryProvider.overrideWithValue(Future.value(recordings)),
      ],
    );
    addTearDown(container.dispose);
    return container;
  }

  test('a recording becomes a note that says it needs transcribing', () async {
    final recorder = FakeRecorder();
    final container = containerWith(recorder);
    final controller = container.read(recordingControllerProvider.notifier);

    expect(await controller.start(), isNull);
    expect(container.read(recordingControllerProvider).isRecording, isTrue);
    expect(recorder.startedAt, endsWith(recordingExtension));

    expect(await controller.stop(), isNull);
    expect(container.read(recordingControllerProvider).isRecording, isFalse);

    final notes = container.read(notesProvider);
    expect(notes, hasLength(1));
    final note = notes.single;
    // The content describes the recording rather than standing in for it:
    // this text is what the search index holds if transcription never runs.
    expect(note.content, startsWith('Voice note, 0:'));
    expect(note.sourceType, 'audio');
    expect(note.metadata[metaAwaitingTranscription], isTrue);
    expect(note.metadata[metaLocalPath], recorder.startedAt);
    expect(note.metadata[metaDurationSeconds], isA<int>());
  });

  test('a refused microphone writes no note and says why', () async {
    final container = containerWith(FakeRecorder(permitted: false));
    final controller = container.read(recordingControllerProvider.notifier);

    expect(await controller.start(), RecordingProblem.noPermission);
    expect(container.read(recordingControllerProvider).isRecording, isFalse);
    expect(
      container.read(recordingControllerProvider).problem,
      RecordingProblem.noPermission,
    );
    expect(container.read(notesProvider), isEmpty);
  });

  test('a recorder that will not start writes no note', () async {
    final container = containerWith(FakeRecorder(failOnStart: true));
    final controller = container.read(recordingControllerProvider.notifier);

    expect(await controller.start(), RecordingProblem.recorderFailed);
    expect(container.read(notesProvider), isEmpty);
  });

  test('a recorder that dies on stop writes no note', () async {
    final container = containerWith(FakeRecorder(failOnStop: true));
    final controller = container.read(recordingControllerProvider.notifier);

    await controller.start();
    expect(await controller.stop(), RecordingProblem.recorderFailed);
    expect(container.read(notesProvider), isEmpty);
  });

  test('an empty recording writes no note and leaves no file', () async {
    final recorder = FakeRecorder(contents: '');
    final container = containerWith(recorder);
    final controller = container.read(recordingControllerProvider.notifier);

    await controller.start();
    expect(await controller.stop(), RecordingProblem.nothingRecorded);
    expect(container.read(notesProvider), isEmpty);
    // The empty file goes rather than being left for an upload that the
    // server would refuse.
    expect(File(recorder.startedAt!).existsSync(), isFalse);
  });

  test('a recording the recorder never wrote writes no note', () async {
    final container = containerWith(FakeRecorder(writesFile: false));
    final controller = container.read(recordingControllerProvider.notifier);

    await controller.start();
    expect(await controller.stop(), RecordingProblem.nothingRecorded);
    expect(container.read(notesProvider), isEmpty);
  });

  test('cancelling throws the recording away', () async {
    final recorder = FakeRecorder();
    final container = containerWith(recorder);
    final controller = container.read(recordingControllerProvider.notifier);

    await controller.start();
    final path = recorder.startedAt!;
    await controller.cancel();

    expect(recorder.cancels, 1);
    expect(File(path).existsSync(), isFalse);
    expect(container.read(notesProvider), isEmpty);
    expect(container.read(recordingControllerProvider).isRecording, isFalse);
  });

  test('stopping when nothing is recording does nothing', () async {
    final container = containerWith(FakeRecorder());
    final controller = container.read(recordingControllerProvider.notifier);

    expect(await controller.stop(), isNull);
    expect(container.read(notesProvider), isEmpty);
  });

  test('starting twice does not start a second recording', () async {
    final recorder = FakeRecorder();
    final container = containerWith(recorder);
    final controller = container.read(recordingControllerProvider.notifier);

    await controller.start();
    final first = recorder.startedAt;
    expect(await controller.start(), isNull);
    expect(recorder.startedAt, first);
  });

  test('describeRecording reads as a duration', () {
    expect(describeRecording(0), 'Voice note, 0:00');
    expect(describeRecording(9), 'Voice note, 0:09');
    expect(describeRecording(74), 'Voice note, 1:14');
    expect(describeRecording(600), 'Voice note, 10:00');
  });
}
