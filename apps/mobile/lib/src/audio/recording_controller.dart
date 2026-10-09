import 'dart:async';

import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/audio/voice_recorder.dart';
import 'package:evomem_mobile/src/log/log.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Metadata keys a recording puts on its note. The server reads the first two
/// (ADR-0016); the third is this device's own business.
const metaAwaitingTranscription = 'awaiting_transcription';

/// How long the recording ran, in whole seconds.
const metaDurationSeconds = 'duration_seconds';

/// How to find the recording on this device: a file path on a phone, a
/// `blob:` URL in a browser (ADR-0029). Not sent anywhere — the server names
/// the recording by the note it belongs to.
const metaLocalPath = 'local_audio_path';

/// Set when the bytes a note stands for are gone and cannot come back.
///
/// In a browser a recording does not survive a reload (ADR-0029). A note
/// left waiting forever for audio nobody has is worse than one that says the
/// audio is gone, so the upload marks it and takes the waiting flag off.
const metaRecordingLost = 'recording_lost';

/// Why a recording did not start.
enum RecordingProblem {
  /// The microphone was refused, or the person has not allowed it.
  noPermission,

  /// Something below this failed: no microphone, a busy one, a disk that
  /// would not take the file.
  recorderFailed,

  /// The recording held no audio. Usually a tap that stopped as soon as it
  /// started.
  nothingRecorded,
}

/// What the recorder is doing, for a screen to show.
class RecordingState {
  /// Nothing is being recorded.
  const new idle() : isRecording = false, startedAt = null, problem = null;

  /// A recording is running, since [startedAt].
  const new recording(DateTime this.startedAt)
    : isRecording = true,
      problem = null;

  /// The last attempt did not work.
  const new failed(RecordingProblem this.problem)
    : isRecording = false,
      startedAt = null;

  /// Whether a recording is running now.
  final bool isRecording;

  /// When the running recording began, or null when none is.
  final DateTime? startedAt;

  /// Why the last attempt failed, or null when it did not.
  final RecordingProblem? problem;
}

/// The recorder this app uses. A test overrides it with a fake.
final voiceRecorderProvider = Provider<VoiceRecorder>(
  (ref) => PackageVoiceRecorder(),
);

/// Starts and stops recordings, and turns a finished one into a note.
///
/// A recording becomes a note whose content *describes* it — `Voice note,
/// 0:14` — marked `awaiting_transcription`, exactly as a Telegram voice
/// message does. Nothing on the phone transcribes; `evomem transcribe` does,
/// after the recording has been uploaded (ADR-0016, ADR-0018).
class RecordingController extends Notifier<RecordingState> {
  VoiceRecorder get _recorder => ref.read(voiceRecorderProvider);
  RecordingStore get _store => ref.read(recordingStoreProvider);
  Logbook get _log => ref.read(logbookProvider.notifier);

  String? _path;

  @override
  RecordingState build() => const RecordingState.idle();

  /// Begins recording, asking for the microphone if it has not been granted.
  ///
  /// Returns the reason it did not start, or null when it did.
  Future<RecordingProblem?> start() async {
    if (state.isRecording) return null;

    final bool allowed;
    try {
      // Object, not Exception. A failure that comes up through browser
      // interop arrives as an Error, which `on Exception` does not catch —
      // so it escaped this handler, took the app's generic message with it
      // and left nothing anywhere to read.
      allowed = await _recorder.hasPermission();
    } on Object catch (e, stack) {
      _log.error('recording', 'asking for the microphone failed', e, stack);
      state = const RecordingState.failed(RecordingProblem.recorderFailed);
      return RecordingProblem.recorderFailed;
    }
    if (!allowed) {
      _log.warn('recording', 'the microphone was refused');
      state = const RecordingState.failed(RecordingProblem.noPermission);
      return RecordingProblem.noPermission;
    }

    // On a phone this is where the file goes. In a browser the recorder
    // ignores it and hands back its own handle when it stops (ADR-0029).
    final path = await _store.newTarget();

    try {
      await _recorder.start(path);
    } on Object catch (e, stack) {
      _log.error('recording', 'the recorder would not start', e, stack);
      state = const RecordingState.failed(RecordingProblem.recorderFailed);
      return RecordingProblem.recorderFailed;
    }

    _log.info('recording', 'started, into $path');
    _path = path;
    state = RecordingState.recording(DateTime.now());
    return null;
  }

  /// Stops recording and writes the note.
  ///
  /// Returns the reason there is no note, or null when one was written.
  Future<RecordingProblem?> stop() async {
    if (!state.isRecording) return null;

    final startedAt = state.startedAt ?? DateTime.now();
    final String? path;
    try {
      path = await _recorder.stop();
    } on Object catch (e, stack) {
      _log.error('recording', 'the recorder would not stop', e, stack);
      state = const RecordingState.failed(RecordingProblem.recorderFailed);
      return RecordingProblem.recorderFailed;
    }

    // What the recorder returned is the handle from here on: a path on a
    // phone, a blob URL in a browser. Claiming it is also where the browser
    // reads the bytes out, so the URL's own lifetime stops mattering.
    final size = path == null ? null : await _store.claim(path);
    if (path == null || size == null || size == 0) {
      _log.warn('recording', 'nothing was captured', 'handle=$path size=$size');
      // Nothing was captured. Whatever was left behind goes rather than
      // waiting for an upload that would be refused.
      if (path != null) await _store.remove(path);
      _path = null;
      state = const RecordingState.failed(RecordingProblem.nothingRecorded);
      return RecordingProblem.nothingRecorded;
    }

    final seconds = DateTime.now().difference(startedAt).inSeconds;
    ref
        .read(notesProvider.notifier)
        .add(
          rawContent: describeRecording(seconds),
          projectId: ref.read(currentProjectProvider),
          sourceType: 'audio',
          metadata: {
            metaAwaitingTranscription: true,
            metaDurationSeconds: seconds,
            metaLocalPath: path,
          },
        );

    _log.info('recording', 'kept $seconds second(s) as a note');
    _path = null;
    state = const RecordingState.idle();
    return null;
  }

  /// Stops recording and throws the file away. No note is written.
  Future<void> cancel() async {
    if (!state.isRecording) return;
    try {
      await _recorder.cancel();
    } on Object catch (e) {
      // Nothing left to do about it; the state still has to go back. Said
      // out loud, because a recorder that will not cancel is usually one
      // that will not start next time either.
      _log.warn('recording', 'the recorder would not cancel', e);
    }
    final path = _path;
    if (path != null) await _store.remove(path);
    _path = null;
    state = const RecordingState.idle();
  }
}

/// What a recording's note says until it is transcribed.
///
/// It describes the recording rather than standing in as a placeholder,
/// because this text is what the search index holds and what a model is shown
/// if the transcription never happens.
String describeRecording(int seconds) {
  final minutes = seconds ~/ 60;
  final remainder = (seconds % 60).toString().padLeft(2, '0');
  return 'Voice note, $minutes:$remainder';
}

/// The recorder's state, for a screen.
final recordingControllerProvider =
    NotifierProvider<RecordingController, RecordingState>(
      RecordingController.new,
    );
