import 'dart:io';

import 'package:evomem_mobile/src/audio/recording_controller.dart';
import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/audio/recording_store_io.dart';
import 'package:evomem_mobile/src/audio/voice_recorder.dart';
import 'package:evomem_mobile/src/log/log.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

import '../sqflite_test_setup.dart' as sqflite_setup;

/// A recorder that refuses the microphone without throwing.
class RefusingRecorder implements VoiceRecorder {
  @override
  Future<bool> hasPermission() async => false;

  @override
  Future<bool> isRecording() async => false;

  @override
  Future<void> start(String path) async {}

  @override
  Future<String?> stop() async => null;

  @override
  Future<void> cancel() async {}

  @override
  Future<void> dispose() async {}
}

/// A recorder that fails the way browser interop does: with an **Error**,
/// not an Exception. `on Exception` does not catch one, which is how a
/// recording could fail in a browser and leave nothing anywhere to read.
class ErroringRecorder implements VoiceRecorder {
  @override
  Future<bool> hasPermission() async => throw StateError('NotAllowedError');

  @override
  Future<bool> isRecording() async => false;

  @override
  Future<void> start(String path) async {}

  @override
  Future<String?> stop() async => null;

  @override
  Future<void> cancel() async {}

  @override
  Future<void> dispose() async {}
}

/// What record_web actually does: its MediaRecorder delegate catches every
/// failure inside its own `start`, prints it, and returns normally. The
/// caller is told recording began; nothing is recording.
///
/// This is the case that produced "started" followed by "nothing was
/// captured — handle=null" in the log, with no error anywhere between them.
class SilentlyDeadRecorder implements VoiceRecorder {
  @override
  Future<bool> hasPermission() async => true;

  @override
  Future<void> start(String path) async {}

  @override
  Future<bool> isRecording() async => false;

  @override
  Future<String?> stop() async => null;

  @override
  Future<void> cancel() async {}

  @override
  Future<void> dispose() async {}
}

void main() {
  sqflite_setup.setupSqfliteFfi();

  late Directory recordings;

  setUp(() async {
    recordings = Directory.systemTemp.createTempSync('evomem-log');
    final db = await DatabaseHelper.instance.database;
    await db.delete('notes');
  });

  tearDown(() {
    if (recordings.existsSync()) recordings.deleteSync(recursive: true);
  });

  ProviderContainer containerWith(VoiceRecorder recorder) {
    final container = ProviderContainer(
      overrides: [
        voiceRecorderProvider.overrideWithValue(recorder),
        recordingStoreProvider.overrideWithValue(
          FileRecordingStore(directory: Future.value(recordings)),
        ),
      ],
    );
    addTearDown(container.dispose);
    return container;
  }

  // The whole point of the logbook. Before it, this failed with a generic
  // snackbar and the reason went nowhere a person could reach.
  test('a recorder that throws an Error is caught and written down', () async {
    final container = containerWith(ErroringRecorder());

    final problem = await container
        .read(recordingControllerProvider.notifier)
        .start();

    expect(problem, RecordingProblem.recorderFailed);

    final entries = container.read(logbookProvider);
    expect(entries, isNotEmpty, reason: 'the failure was swallowed again');
    expect(entries.first.level, LogLevel.error);
    expect(entries.first.source, 'recording');
    expect(
      entries.first.detail,
      contains('NotAllowedError'),
      reason: 'the reason did not survive',
    );
  });

  test('a refused microphone says so without an error', () async {
    final container = containerWith(RefusingRecorder());

    final problem = await container
        .read(recordingControllerProvider.notifier)
        .start();

    expect(problem, RecordingProblem.noPermission);
    final entry = container.read(logbookProvider).first;
    expect(entry.level, LogLevel.warning);
    expect(entry.message, contains('refused'));
  });

  // A start that returns is not a start that worked. Before this, the app
  // said "started", the user waited, and stop() handed back nothing with no
  // error anywhere — which is exactly what the log showed.
  test('a recorder that dies silently is caught at start', () async {
    final container = containerWith(SilentlyDeadRecorder());

    final problem = await container
        .read(recordingControllerProvider.notifier)
        .start();

    expect(problem, RecordingProblem.recorderFailed);
    expect(
      container.read(recordingControllerProvider).isRecording,
      isFalse,
      reason: 'the app believes it is recording when nothing is',
    );

    final entry = container.read(logbookProvider).first;
    expect(entry.level, LogLevel.error);
    expect(entry.message, contains('not recording'));
  });
}
