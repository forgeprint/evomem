import 'dart:async';

import 'package:evomem_mobile/l10n/app_localizations.dart';
import 'package:evomem_mobile/src/audio/recording_controller.dart';
import 'package:evomem_mobile/src/routing/routes.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/rules/note_rules.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

/// The list of notes, with a field to add one and a search bar.
class NotesListScreen extends ConsumerStatefulWidget {
  /// Creates the screen.
  const new({super.key});

  @override
  ConsumerState<NotesListScreen> createState() => _NotesListScreenState();
}

class _NotesListScreenState extends ConsumerState<NotesListScreen> {
  final TextEditingController _searchController = TextEditingController();
  final TextEditingController _addController = TextEditingController();
  NoteProblem? _problem;
  String _searchQuery = '';

  @override
  void dispose() {
    _searchController.dispose();
    _addController.dispose();
    super.dispose();
  }

  void _showRecordingProblem(RecordingProblem problem) {
    if (!mounted) return;
    final l10n = AppLocalizations.of(context);
    final message = switch (problem) {
      RecordingProblem.noPermission => l10n.recordingNeedsMicrophone,
      RecordingProblem.recorderFailed => l10n.recordingFailed,
      RecordingProblem.nothingRecorded => l10n.recordingWasEmpty,
    };
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  void _addNote() {
    final problem = ref
        .read(notesProvider.notifier)
        .add(
          rawContent: _addController.text,
          projectId: ref.read(currentProjectProvider),
          sourceType: ref.read(currentSourceProvider),
        );
    setState(() => _problem = problem);
    if (problem == null) {
      _addController.clear();
      ScaffoldMessenger.of(context).showSnackBar(
        SnackBar(content: Text(AppLocalizations.of(context).noteSaved)),
      );
    }
  }

  String? _errorText(AppLocalizations l10n) => switch (_problem) {
    null => null,
    NoteProblem.blank => l10n.noteBlank,
    NoteProblem.tooLong => l10n.noteTooLong(maxNoteLength),
  };

  List<Note> _filteredNotes(List<Note> notes) {
    if (_searchQuery.isEmpty) return notes;
    final query = _searchQuery.toLowerCase();
    return notes.where((n) => n.content.toLowerCase().contains(query)).toList();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final notes = ref.watch(notesProvider);
    final filtered = _filteredNotes(notes);

    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.homeScreenTitle),
        actions: [
          IconButton(
            tooltip: 'Sources',
            icon: const Icon(Icons.cloud_download_outlined),
            onPressed: () => context.go(const SourcesRoute().location),
          ),
          IconButton(
            tooltip: 'Groups',
            icon: const Icon(Icons.workspaces_outline),
            onPressed: () => context.go(const ClustersRoute().location),
          ),
          IconButton(
            tooltip: l10n.settings,
            icon: const Icon(Icons.settings),
            onPressed: () => context.go(const SettingsRoute().location),
          ),
          IconButton(
            tooltip: l10n.syncStatus,
            icon: const Icon(Icons.sync),
            onPressed: () => context.go(const SyncStatusRoute().location),
          ),
        ],
      ),
      body: Column(
        children: [
          // Search bar
          Padding(
            padding: const EdgeInsets.all(16),
            child: TextField(
              controller: _searchController,
              decoration: InputDecoration(
                hintText: l10n.searchHint,
                prefixIcon: const Icon(Icons.search),
                border: const OutlineInputBorder(),
                suffixIcon: _searchQuery.isNotEmpty
                    ? IconButton(
                        icon: const Icon(Icons.clear),
                        onPressed: () {
                          _searchController.clear();
                          setState(() => _searchQuery = '');
                        },
                      )
                    : null,
              ),
              onChanged: (value) => setState(() => _searchQuery = value),
            ),
          ),
          // Add note field
          Padding(
            padding: const EdgeInsets.symmetric(horizontal: 16),
            child: Column(
              crossAxisAlignment: CrossAxisAlignment.stretch,
              children: [
                TextField(
                  controller: _addController,
                  onSubmitted: (_) => _addNote(),
                  maxLines: 3,
                  minLines: 1,
                  decoration: InputDecoration(
                    labelText: l10n.noteContentLabel,
                    errorText: _errorText(l10n),
                    border: const OutlineInputBorder(),
                  ),
                ),
                const SizedBox(height: 12),
                Row(
                  children: [
                    Expanded(
                      child: ElevatedButton(
                        onPressed: _addNote,
                        child: Text(l10n.addNote),
                      ),
                    ),
                    const SizedBox(width: 12),
                    _RecordButton(onProblem: _showRecordingProblem),
                  ],
                ),
              ],
            ),
          ),
          const SizedBox(height: 12),
          // Notes list
          Expanded(
            child: filtered.isEmpty
                ? Center(child: Text(l10n.noNotes))
                : ListView.builder(
                    itemCount: filtered.length,
                    itemBuilder: (context, index) {
                      final note = filtered[index];
                      return _NoteRow(
                        key: ValueKey(note.id),
                        note: note,
                        onTap: () =>
                            context.go(NoteDetailRoute(note.id).location),
                        onDelete: () => unawaited(_showDeleteDialog(note.id)),
                        onTogglePin: () =>
                            ref.read(notesProvider.notifier).togglePin(note.id),
                      );
                    },
                  ),
          ),
        ],
      ),
    );
  }

  Future<void> _showDeleteDialog(String noteId) async {
    final l10n = AppLocalizations.of(context);
    return await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: Text(l10n.deleteNote),
        content: Text(l10n.deleteConfirm),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(l10n.cancel),
          ),
          TextButton(
            onPressed: () {
              ref.read(notesProvider.notifier).delete(noteId);
              Navigator.of(context).pop();
              ScaffoldMessenger.of(context)
                  .showSnackBar(SnackBar(content: Text(l10n.noteDeleted)));
            },
            child: const Text('Delete', style: TextStyle(color: Colors.red)),
          ),
        ],
      ),
    );
  }
}

/// One row: a widget class rather than a `_buildRow` method, so that it has
/// an element of its own and rebuilds only when its own note changes.
class _NoteRow extends StatelessWidget {
  const new({
    required this.note,
    required this.onTap,
    required this.onDelete,
    required this.onTogglePin,
    super.key,
  });

  final Note note;
  final VoidCallback onTap;
  final VoidCallback onDelete;
  final VoidCallback onTogglePin;

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);
    final isPinned = note.metadata['pinned'] as bool? ?? false;

    return Dismissible(
      key: ValueKey(note.id),
      direction: DismissDirection.endToStart,
      background: Container(
        alignment: Alignment.centerRight,
        padding: const EdgeInsets.only(right: 20),
        color: Colors.red,
        child: const Icon(Icons.delete, color: Colors.white),
      ),
      confirmDismiss: (_) async {
        onDelete();
        return false; // We handle deletion in the dialog
      },
      child: ListTile(
        leading: isPinned
            ? const Icon(Icons.push_pin, color: Colors.amber)
            : null,
        title: Text(note.content, maxLines: 2, overflow: TextOverflow.ellipsis),
        subtitle: Text(
          '${note.sourceType} • ${_formatDate(note.updatedAt)}',
          style: Theme.of(context).textTheme.bodySmall,
        ),
        trailing: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            IconButton(
              icon: Icon(isPinned ? Icons.push_pin : Icons.push_pin_outlined),
              tooltip: isPinned ? 'Unpin' : 'Pin',
              onPressed: onTogglePin,
            ),
            IconButton(
              icon: const Icon(Icons.chevron_right),
              tooltip: l10n.openNote,
              onPressed: onTap,
            ),
          ],
        ),
        onTap: onTap,
      ),
    );
  }

  String _formatDate(DateTime date) {
    final hour = date.hour.toString().padLeft(2, '0');
    final minute = date.minute.toString().padLeft(2, '0');
    return '${date.day}.${date.month}.${date.year} $hour:$minute';
  }
}

/// Records a voice note: one tap to start, one to stop.
///
/// The note it leaves behind describes the recording and is marked as
/// awaiting a transcript; nothing here transcribes. A long tap cancels,
/// which is the only way to throw a recording away before it becomes a note.
class _RecordButton extends ConsumerWidget {
  const new({required this.onProblem});

  final void Function(RecordingProblem) onProblem;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final recording = ref.watch(recordingControllerProvider).isRecording;
    final controller = ref.read(recordingControllerProvider.notifier);

    return Tooltip(
      message: recording
          ? l10n.stopRecordingTooltip
          : l10n.recordVoiceNoteTooltip,
      child: GestureDetector(
        onLongPress: recording ? controller.cancel : null,
        child: FilledButton.tonalIcon(
          onPressed: () async {
            final problem = recording
                ? await controller.stop()
                : await controller.start();
            if (problem != null) onProblem(problem);
          },
          icon: Icon(recording ? Icons.stop : Icons.mic),
          label: Text(recording ? l10n.stopRecording : l10n.recordVoiceNote),
        ),
      ),
    );
  }
}
