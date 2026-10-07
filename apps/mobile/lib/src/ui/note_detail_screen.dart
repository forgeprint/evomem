import 'dart:async';

import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';
import 'package:evomem_mobile/l10n/app_localizations.dart';
import 'package:evomem_mobile/src/routing/routes.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/rules/note_rules.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';

/// One note, reached by a link such as `/notes/abc123`.
class NoteDetailScreen extends ConsumerWidget {
  /// Creates the screen for the note with this [noteId].
  const NoteDetailScreen({required this.noteId, super.key});

  /// The id parsed out of the link.
  final String noteId;

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final l10n = AppLocalizations.of(context);
    final notes = ref.watch(notesProvider);
    final note = notes.where((n) => n.id == noteId).firstOrNull;

    if (note == null) {
      return Scaffold(
        appBar: AppBar(title: Text(l10n.appTitle)),
        body: Center(
          child: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              Text(l10n.noteNotFound),
              const SizedBox(height: 12),
              ElevatedButton(
                onPressed: () => context.go(const NotesListRoute().location),
                child: Text(l10n.backToList),
              ),
            ],
          ),
        ),
      );
    }

    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.appTitle),
        actions: [
          IconButton(
            icon: const Icon(Icons.edit),
            tooltip: 'Edit',
            onPressed: () => unawaited(_showEditDialog(context, ref, note)),
          ),
          IconButton(
            icon: Icon(
              note.metadata['pinned'] as bool? ?? false
                  ? Icons.push_pin
                  : Icons.push_pin_outlined,
            ),
            tooltip: 'Toggle pin',
            onPressed: () => ref.read(notesProvider.notifier).togglePin(note.id),
          ),
          IconButton(
            icon: const Icon(Icons.delete_outline),
            tooltip: 'Delete',
            onPressed: () => unawaited(_showDeleteDialog(context, ref, note.id)),
          ),
        ],
      ),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Row(
              children: [
                Chip(
                  label: Text(note.sourceType),
                  backgroundColor: _sourceTypeColor(note.sourceType),
                ),
                const SizedBox(width: 8),
                Text(
                  _formatDateTime(note.updatedAt),
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ],
            ),
            const SizedBox(height: 16),
            Text(
              note.content,
              style: Theme.of(context).textTheme.bodyLarge,
            ),
            if (note.metadata.isNotEmpty) ...[
              const SizedBox(height: 16),
              const Divider(),
              const SizedBox(height: 8),
              Text(
                'Metadata',
                style: Theme.of(context).textTheme.titleSmall,
              ),
              const SizedBox(height: 8),
              ...note.metadata.entries.map((e) => Padding(
                padding: const EdgeInsets.symmetric(vertical: 2),
                child: Text('${e.key}: ${e.value}'),
              )),
            ],
            const SizedBox(height: 24),
            SizedBox(
              width: double.infinity,
              child: ElevatedButton(
                onPressed: () => context.go(const NotesListRoute().location),
                child: Text(l10n.backToList),
              ),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _showEditDialog(
    BuildContext context,
    WidgetRef ref,
    Note note,
  ) async {
    final l10n = AppLocalizations.of(context);
    final controller = TextEditingController(text: note.content);
    NoteProblem? problem;

    return showDialog<void>(
      context: context,
      builder: (context) => StatefulBuilder(
        builder: (context, setState) => AlertDialog(
          title: const Text('Edit Note'),
          content: Column(
            mainAxisSize: MainAxisSize.min,
            children: [
              TextField(
                controller: controller,
                maxLines: 5,
                minLines: 3,
                decoration: InputDecoration(
                  labelText: l10n.noteContentLabel,
                  errorText: problem != null
                      ? (problem == NoteProblem.blank
                          ? l10n.noteBlank
                          : l10n.noteTooLong(maxNoteLength))
                      : null,
                  border: const OutlineInputBorder(),
                ),
                onChanged: (_) => setState(() => problem = checkNoteContent(controller.text)),
              ),
            ],
          ),
          actions: [
            TextButton(
              onPressed: () => Navigator.of(context).pop(),
              child: Text(l10n.cancel),
            ),
            ElevatedButton(
              onPressed: problem == null
                  ? () {
                      ref.read(notesProvider.notifier).update(
                        id: note.id,
                        newContent: controller.text,
                      );
                      Navigator.of(context).pop();
                      ScaffoldMessenger.of(context).showSnackBar(
                        SnackBar(content: Text(l10n.noteSaved)),
                      );
                    }
                  : null,
              child: Text(l10n.saveNote),
            ),
          ],
        ),
      ),
    );
  }

  Future<void> _showDeleteDialog(
    BuildContext context,
    WidgetRef ref,
    String noteId,
  ) async {
    final l10n = AppLocalizations.of(context);
    return showDialog<void>(
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
              context.go(const NotesListRoute().location);
              ScaffoldMessenger.of(context).showSnackBar(
                SnackBar(content: Text(l10n.noteDeleted)),
              );
            },
            child: Text('Delete', style: TextStyle(color: Colors.red)),
          ),
        ],
      ),
    );
  }

  Color _sourceTypeColor(String sourceType) {
    switch (sourceType) {
      case 'telegram':
        return Colors.blue.shade100;
      case 'jira':
        return Colors.orange.shade100;
      case 'shortcut':
        return Colors.green.shade100;
      case 'audio':
        return Colors.purple.shade100;
      case 'mcp':
        return Colors.teal.shade100;
      default:
        return Colors.grey.shade100;
    }
  }

  String _formatDateTime(DateTime date) {
    return '${date.day}.${date.month}.${date.year} '
        '${date.hour.toString().padLeft(2, '0')}:${date.minute.toString().padLeft(2, '0')}';
  }
}