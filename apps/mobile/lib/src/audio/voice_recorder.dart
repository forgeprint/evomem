import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:record/record.dart';

/// Recording a voice note, behind an interface.
///
/// The implementation talks to a microphone, which no test can do. Everything
/// above this — when a note is written, what its content says, what happens
/// when permission is refused — is tested against a fake.
abstract interface class VoiceRecorder {
  /// Whether the microphone may be used, asking the person if they have not
  /// been asked yet.
  Future<bool> hasPermission();

  /// Starts recording into [path].
  Future<void> start(String path);

  /// Whether a recording is actually running.
  ///
  /// Asked after [start], because a start that returns is not a start that
  /// worked: record_web's MediaRecorder delegate catches every failure in
  /// its own `start`, prints it and returns normally, leaving the caller
  /// told that recording began when nothing is (ADR-0029).
  Future<bool> isRecording();

  /// Stops recording and returns the file, or null when nothing was written.
  Future<String?> stop();

  /// Stops recording and discards the file.
  Future<void> cancel();

  /// Releases the microphone.
  Future<void> dispose();
}

/// The settings every recording is made with.
///
/// Mono at 16 kHz and 32 kbit/s, against the package's defaults of stereo at
/// 44.1 kHz and 128 kbit/s. Three reasons, all pointing the same way: a
/// transcription model resamples to 16 kHz mono before it does anything else,
/// so the extra data is discarded on arrival; the upload is bounded at 20 MiB
/// (ADR-0018), which the defaults reach in about twenty minutes and these
/// settings in about an hour and a half; and a phone on a tunnel is the
/// slowest link in the chain.
///
/// The encoder, which is not the same answer on every platform.
///
/// A phone writes AAC-LC, the package's default, into an `.m4a`.
///
/// A browser is asked for Opus instead. Asking it for AAC goes through
/// MediaRecorder, whose support for `audio/mp4` is uneven — and record_web's
/// delegate swallows a failed start, so the result is a recorder that
/// reports success and produces nothing. Opus in a WebM container is what
/// browsers have always encoded; `shared/audio` already maps `audio/webm`,
/// and the OpenAI speech-to-text interface documents webm as supported
/// (ADR-0016, ADR-0029).
const AudioEncoder _encoder = kIsWeb ? AudioEncoder.opus : AudioEncoder.aacLc;

/// The settings every recording is made with.
const speechRecordConfig = RecordConfig(
  encoder: _encoder,
  numChannels: 1,
  sampleRate: 16000,
  bitRate: 32000,
);

/// What a recording is called on this platform.
///
/// Only the phone's name matters: in a browser the recorder ignores the path
/// entirely and the type comes back with the bytes (ADR-0029).
const recordingExtension = kIsWeb ? '.webm' : '.m4a';

/// What `POST /ingest/audio` is told the body is, when nothing better is
/// known. The browser reports the blob's own type instead.
const recordingMediaType = kIsWeb ? 'audio/webm' : 'audio/m4a';

/// [VoiceRecorder] over the `record` package.
///
/// Verified against record 7.1.1 (pub.dev, read 2026-10-08): `AudioRecorder`
/// with `hasPermission`, `start(RecordConfig, path:)`, `isRecording()`,
/// `stop()` returning the path, `cancel()` and `dispose()`.
///
///   https://pub.dev/packages/record
class PackageVoiceRecorder implements VoiceRecorder {
  final AudioRecorder _recorder = AudioRecorder();

  @override
  Future<bool> hasPermission() => _recorder.hasPermission();

  @override
  Future<void> start(String path) =>
      _recorder.start(speechRecordConfig, path: path);

  @override
  Future<bool> isRecording() => _recorder.isRecording();

  @override
  Future<String?> stop() => _recorder.stop();

  @override
  Future<void> cancel() => _recorder.cancel();

  @override
  Future<void> dispose() async => await _recorder.dispose();
}
