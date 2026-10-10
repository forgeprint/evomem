import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/audio/voice_recorder.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  // The two platforms record different things on purpose (ADR-0029) and
  // the server keeps whichever arrived. So a note made in a browser can be
  // opened on a phone, and that is where the formats meet.
  group('what a device will play', () {
    test('a phone plays what a phone recorded', () {
      expect(
        playbackRefusal('audio/m4a', onApplePlatform: true),
        isNull,
        reason: 'this is exactly what this app records on a phone',
      );
    });

    test('an Apple platform refuses WebM, and says why', () {
      final refusal = playbackRefusal('audio/webm', onApplePlatform: true);

      expect(refusal, isNotNull);
      expect(refusal, contains('browser'));
      expect(
        refusal,
        contains('still plays in a browser'),
        reason: 'a refusal that does not say where it does work is half an '
            'answer',
      );
    });

    test('the codec parameter the browser adds does not hide the type', () {
      // MediaRecorder reports `audio/webm;codecs=opus`, which is what the
      // browser's own blob carries and what the upload declares.
      expect(
        playbackRefusal('audio/webm;codecs=opus', onApplePlatform: true),
        isNotNull,
        reason: 'the type was compared whole and the refusal was skipped',
      );
    });

    test('elsewhere WebM is not refused here', () {
      // Android reads WebM. The refusal is Apple's, not this app's opinion
      // about the format.
      expect(playbackRefusal('audio/webm', onApplePlatform: false), isNull);
    });

    test('what this app records on a phone is not what it records on web', () {
      // If these ever become the same thing, the refusal above stops
      // mattering — and that is worth noticing rather than discovering.
      expect(recordingMediaType, anyOf('audio/m4a', 'audio/webm'));
    });

    test('a failure says its message, not its class name', () {
      // A release build has no class names. The last time this was got
      // wrong, the log said `Instance of 'minified:fn'`.
      expect(const PlaybackFailure('no').toString(), 'no');
    });
  });

  // AVFoundation reads a local file's type from its extension. Handing it a
  // valid m4a with no extension failed with CoreMedia err=-12864 on an iOS
  // 27 simulator (2026-10-10), which is why this app names the file itself
  // instead of letting the player package hash the bytes.
  group('what a recording is called when it is written out to play', () {
    test('what the server says about a phone recording', () {
      // `GET /notes/{id}/audio` answers audio/mp4 for an m4a, which is the
      // case that failed.
      expect(playbackExtension('audio/mp4'), '.m4a');
      expect(playbackExtension('audio/m4a'), '.m4a');
    });

    test('what a browser recording is called', () {
      expect(playbackExtension('audio/webm;codecs=opus'), '.webm');
    });

    test('the parameter and the case do not change the answer', () {
      expect(playbackExtension('AUDIO/MP4; codecs="mp4a.40.2"'), '.m4a');
    });

    test('an unknown type still gets an extension', () {
      // None at all is the one thing measured to fail.
      expect(playbackExtension('application/octet-stream'), startsWith('.'));
    });

    test('the two this app records both map', () {
      expect(playbackExtension(recordingMediaType), isNot('.'));
      expect(playbackExtension('audio/m4a'), '.m4a');
      expect(playbackExtension('audio/webm'), '.webm');
    });
  });
}
