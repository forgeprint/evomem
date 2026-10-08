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
/// AAC-LC is the package's default encoder and writes an `.m4a`, which the
/// transcription interface accepts and `shared/audio` stores as `audio/m4a`.
const speechRecordConfig = RecordConfig(
  // AudioEncoder.aacLc is the package default and so not repeated here.
  numChannels: 1,
  sampleRate: 16000,
  bitRate: 32000,
);

/// The recording extension and the media type that goes with it, kept
/// together because the server decides how to decode by the second and names
/// the file by the first.
const recordingExtension = '.m4a';

/// What `POST /ingest/audio` is told the body is.
const recordingMediaType = 'audio/m4a';

/// [VoiceRecorder] over the `record` package.
///
/// Verified against record 7.1.1 (pub.dev, read 2026-10-08): `AudioRecorder`
/// with `hasPermission`, `start(RecordConfig, path:)`, `stop()` returning the
/// path, `cancel()` and `dispose()`.
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
  Future<String?> stop() => _recorder.stop();

  @override
  Future<void> cancel() => _recorder.cancel();

  @override
  Future<void> dispose() async => await _recorder.dispose();
}
