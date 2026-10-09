import 'package:evomem_mobile/src/audio/recording_controller.dart';
import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:evomem_mobile/src/log/log.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Plays back the recording a note stands for.
///
/// Shown only when there is something to play and this build can play it: a
/// note with no recording has nothing, a recording whose bytes are gone
/// cannot be played, and a phone has no player yet (ADR-0029).
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

  String? get _handle {
    final handle = widget.note.metadata[metaLocalPath];
    return handle is String && handle.isNotEmpty ? handle : null;
  }

  Future<void> _toggle() async {
    final handle = _handle;
    if (handle == null) return;
    final player = ref.read(recordingPlayerProvider);

    if (_playing) {
      await player.stop();
      if (mounted) setState(() => _playing = false);
      return;
    }

    // Checked before playing: the bytes can be gone — removed, or recorded
    // in a browser that has since been reloaded — and an element pointed at
    // a dead blob fails in a way nobody can read.
    final recording = await ref.read(recordingStoreProvider).read(handle);
    if (recording == null) {
      ref
          .read(logbookProvider.notifier)
          .warn('playback', 'the recording is gone', 'handle=$handle');
      if (!mounted) return;
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text(
            'That recording is gone. It does not survive a reload.',
          ),
        ),
      );
      return;
    }

    setState(() => _playing = true);
    try {
      await ref.read(recordingPlayerProvider).play(handle);
    } on Object catch (e) {
      ref.read(logbookProvider.notifier).error('playback', 'could not play', e);
      if (!mounted) return;
      ScaffoldMessenger.of(context)
          .showSnackBar(SnackBar(content: Text('Could not play it: $e')));
    } finally {
      if (mounted) setState(() => _playing = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    if (_handle == null || !ref.read(recordingPlayerProvider).canPlay) {
      return const SizedBox.shrink();
    }
    return Align(
      alignment: Alignment.centerLeft,
      child: FilledButton.tonalIcon(
        onPressed: _toggle,
        icon: Icon(_playing ? Icons.stop : Icons.play_arrow),
        label: Text(_playing ? 'Stop' : 'Play the recording'),
      ),
    );
  }
}
