import 'dart:async';
import 'dart:io';

import 'package:evomem_mobile/src/audio/voice_recorder.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:path/path.dart' as p;
import 'package:path_provider/path_provider.dart';

/// Metadata keys a recording puts on its note. The server reads the first two
/// (ADR-0016); the third is this device's own business.
const metaAwaitingTranscription = 'awaiting_transcription';

/// How long the recording ran, in whole seconds.
const metaDurationSeconds = 'duration_seconds';

/// Where the file is on this phone. Not sent anywhere: the server names the
/// recording by the note it belongs to.
const metaLocalPath = 'local_audio_path';

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

/// Where recordings are written. A test overrides it with a temporary
/// directory, because `path_provider` needs a platform to answer.
final recordingsDirectoryProvider = Provider<Future<Directory>>((ref) async {
  final documents = await getApplicationDocumentsDirectory();
  return Directory(p.join(documents.path, 'recordings'));
});

/// Starts and stops recordings, and turns a finished one into a note.
///
/// A recording becomes a note whose content *describes* it — `Voice note,
/// 0:14` — marked `awaiting_transcription`, exactly as a Telegram voice
/// message does. Nothing on the phone transcribes; `evomem transcribe` does,
/// after the recording has been uploaded (ADR-0016, ADR-0018).
class RecordingController extends Notifier<RecordingState> {
  VoiceRecorder get _recorder => ref.read(voiceRecorderProvider);

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
      allowed = await _recorder.hasPermission();
    } on Exception {
      state = const RecordingState.failed(RecordingProblem.recorderFailed);
      return RecordingProblem.recorderFailed;
    }
    if (!allowed) {
      state = const RecordingState.failed(RecordingProblem.noPermission);
      return RecordingProblem.noPermission;
    }

    final directory = await ref.read(recordingsDirectoryProvider);
    await directory.create(recursive: true);
    // Named by when it was made, which is unique enough on one device and
    // readable in a file listing. The note's own identifier is not available
    // yet: it is minted when the recording stops.
    final path = p.join(
      directory.path,
      'note-${DateTime.now().millisecondsSinceEpoch}$recordingExtension',
    );

    try {
      await _recorder.start(path);
    } on Exception {
      state = const RecordingState.failed(RecordingProblem.recorderFailed);
      return RecordingProblem.recorderFailed;
    }

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
    } on Exception {
      state = const RecordingState.failed(RecordingProblem.recorderFailed);
      return RecordingProblem.recorderFailed;
    }

    final file = path == null ? null : File(path);
    if (file == null || !file.existsSync() || await file.length() == 0) {
      // Nothing was captured. The empty file, if there is one, goes rather
      // than being left for an upload that would be refused.
      if (file != null && file.existsSync()) {
        await file.delete();
      }
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

    _path = null;
    state = const RecordingState.idle();
    return null;
  }

  /// Stops recording and throws the file away. No note is written.
  Future<void> cancel() async {
    if (!state.isRecording) return;
    try {
      await _recorder.cancel();
    } on Exception {
      // Nothing left to do about it; the state still has to go back.
    }
    final path = _path;
    if (path != null) {
      final file = File(path);
      if (file.existsSync()) await file.delete();
    }
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
