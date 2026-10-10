import 'package:evomem_mobile/src/audio/recording_controller.dart';
import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:evomem_mobile/src/http/utf8_body.dart';
import 'package:evomem_mobile/src/log/log.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Plays back the recording a note stands for.
///
/// Shown when the note stands for a recording and this build can play one.
/// A phone has no player yet, and a note with no audio has nothing to play
/// (ADR-0029).
class PlayRecordingButton extends ConsumerStatefulWidget {
  /// Creates the button for [note].
  const new(this.note, {super.key});

  /// The note whose recording to play.
  final Note note;

  @override
  ConsumerState<PlayRecordingButton> createState() =>
      _PlayRecordingButtonState();
}

class _PlayRecordingButtonState extends ConsumerState<PlayRecordingButton> {
  bool _playing = false;
  bool _fetching = false;

  String? get _handle {
    final handle = widget.note.metadata[metaLocalPath];
    return handle is String && handle.isNotEmpty ? handle : null;
  }

  bool get _standsForARecording =>
      _handle != null ||
      widget.note.metadata[metaAwaitingTranscription] == true ||
      widget.note.metadata[metaTranscribed] == true;

  Future<void> _toggle() async {
    final player = ref.read(recordingPlayerProvider);

    if (_playing) {
      await player.stop();
      if (mounted) setState(() => _playing = false);
      return;
    }

    setState(() => _fetching = true);
    final RecordingBytes? recording;
    try {
      recording = await _find();
    } finally {
      if (mounted) setState(() => _fetching = false);
    }

    if (recording == null) {
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(
            'There is nothing to play. A recording made in a browser does '
            'not survive a reload; syncing keeps it.',
          ),
        ),
      );
      return;
    }

    setState(() => _playing = true);
    try {
      await player.play(recording);
    } on Object catch (e) {
      ref.read(logbookProvider.notifier).error('playback', 'could not play', e);
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('Could not play it: $e')));
    } finally {
      if (mounted) setState(() => _playing = false);
    }
  }

  /// The bytes, from this device if it still has them and from the server
  /// if it does not.
  ///
  /// The device's copy is the only one before a sync, and after one the
  /// server's outlives a reload — which is why the endpoint exists.
  Future<RecordingBytes?> _find() async {
    final log = ref.read(logbookProvider.notifier);

    final handle = _handle;
    if (handle != null) {
      final local = await ref.read(recordingStoreProvider).read(handle);
      if (local != null) return local;
      log.info('playback', 'no local bytes; asking the server');
    }

    final remoteId = widget.note.remoteId;
    if (remoteId.isEmpty) {
      log.warn('playback', 'the recording is gone and was never synced');
      return null;
    }

    final settings = await ref.read(serverSettingsProvider.future);
    if (settings.url.isEmpty || settings.token.isEmpty) {
      log.warn('playback', 'no server configured to ask');
      return null;
    }

    try {
      // Fetched rather than handed to an audio element as a URL, so the
      // token stays in a header. A query string is the wrong place for it
      // and an audio element cannot send one (ADR-0010).
      final response = await ref
          .read(httpClientProvider)
          .get(
            Uri.parse('${settings.url}/notes/$remoteId/audio'),
            headers: {'Authorization': 'Bearer ${settings.token}'},
          )
          .timeout(const Duration(seconds: 30));

      if (response.statusCode != 200) {
        log.warn(
          'playback',
          'the server would not give the recording',
          'status=${response.statusCode} ${utf8Body(response).trim()}',
        );
        return null;
      }
      return RecordingBytes(
        response.bodyBytes,
        response.headers['content-type'] ?? 'application/octet-stream',
      );
    } on Object catch (e) {
      log.error('playback', 'could not reach the server for it', e);
      return null;
    }
  }

  @override
  Widget build(BuildContext context) {
    if (!_standsForARecording || !ref.read(recordingPlayerProvider).canPlay) {
      return const SizedBox.shrink();
    }
    return Align(
      alignment: Alignment.centerLeft,
      child: FilledButton.tonalIcon(
        onPressed: _fetching ? null : _toggle,
        icon: _fetching
            ? const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : Icon(_playing ? Icons.stop : Icons.play_arrow),
        label: Text(
          _fetching
              ? 'Fetching…'
              : _playing
              ? 'Stop'
              : 'Play the recording',
        ),
      ),
    );
  }
}
