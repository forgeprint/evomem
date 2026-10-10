import 'dart:async';
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

/// What record does on an iOS device whose audio input cannot be opened:
/// `start` never completes and never throws. Measured on an iOS 27
/// simulator on 2026-10-10, where CoreAudio refuses the input device — the
/// button went dead, the log stayed empty, and the only sign anything had
/// happened was a line in the system log.
class HangingRecorder implements VoiceRecorder {
  /// Whether anybody tried to put the recorder back.
  bool cancelled = false;

  @override
  Future<bool> hasPermission() async => true;

  @override
  Future<void> start(String path) => Completer<void>().future;

  @override
  Future<bool> isRecording() async => false;

  @override
  Future<String?> stop() async => null;

  @override
  Future<void> cancel() async => cancelled = true;

  @override
  Future<void> dispose() async {}
}

/// A store that cannot say where a recording should go.
///
/// `getApplicationDocumentsDirectory` can fail, and the call to it used to
/// sit outside every try in start(): the exception left the method without
/// touching the state, the log, or the message the screen shows.
class UnusableStore implements RecordingStore {
  @override
  Future<String> newTarget() async => throw const FileSystemException('full');

  @override
  Future<int?> claim(String handle) async => null;

  @override
  Future<RecordingBytes?> read(String handle) async => null;

  @override
  Future<void> remove(String handle) async {}
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

  // The button that would not work and would not say why — the same shape
  // of failure the logbook was built for in a browser, found again on a
  // phone. A recorder that hangs is now a failure with a reason, after a
  // bounded wait.
  test('a recorder that hangs gives up and says so', () async {
    final recorder = HangingRecorder();
    final container = containerWith(recorder);

    final problem = await container
        .read(recordingControllerProvider.notifier)
        .start();

    expect(problem, RecordingProblem.recorderFailed);
    expect(container.read(recordingControllerProvider).isRecording, isFalse);

    final entry = container.read(logbookProvider).first;
    expect(entry.level, LogLevel.error);
    expect(entry.message, contains('never started'));
    expect(
      recorder.cancelled,
      isTrue,
      reason: 'a half-open recorder makes the next attempt fail too',
    );
  }, timeout: const Timeout(Duration(seconds: 30)));

  test('nowhere to put the recording is a reported failure', () async {
    final container = ProviderContainer(
      overrides: [
        voiceRecorderProvider.overrideWithValue(SilentlyDeadRecorder()),
        recordingStoreProvider.overrideWithValue(UnusableStore()),
      ],
    );
    addTearDown(container.dispose);

    final problem = await container
        .read(recordingControllerProvider.notifier)
        .start();

    expect(
      problem,
      RecordingProblem.recorderFailed,
      reason: 'the exception escaped start() and the screen said nothing',
    );
    final entry = container.read(logbookProvider).first;
    expect(entry.level, LogLevel.error);
    expect(entry.message, contains('nowhere to put'));
  });
}
