import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:evomem_mobile/src/storage/notes_store.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Which project the app is looking at, and how to change it.
///
/// It sits in the app bar rather than behind a settings page because every
/// list on the screen is scoped to it: a filter nobody can see is one people
/// forget is on, and then wonder where their notes went.
class ProjectPicker extends ConsumerWidget {
  /// Creates the picker.
  const new({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final current = ref.watch(currentProjectProvider);
    final projects = ref.watch(projectsProvider);

    return PopupMenuButton<String>(
      tooltip: 'Switch project',
      onSelected: (value) async {
        if (value != _newProject) {
          ref.read(currentProjectProvider.notifier).select(value);
          return;
        }
        final name = await _askForName(context);
        if (name != null && name.isNotEmpty) {
          ref.read(currentProjectProvider.notifier).select(name);
        }
      },
      itemBuilder: (context) => [
        // The known projects come from the notes themselves, so a project
        // exists for exactly as long as something is filed under it.
        for (final project in projects.asData?.value ?? const <ProjectCount>[])
          PopupMenuItem(
            value: project.projectId,
            child: Row(
              children: [
                Icon(
                  project.projectId == current
                      ? Icons.radio_button_checked
                      : Icons.radio_button_unchecked,
                  size: 18,
                ),
                const SizedBox(width: 12),
                Expanded(child: Text(project.projectId)),
                const SizedBox(width: 8),
                Text(
                  '${project.notes}',
                  style: Theme.of(context).textTheme.bodySmall,
                ),
              ],
            ),
          ),
        const PopupMenuDivider(),
        const PopupMenuItem(
          value: _newProject,
          child: Row(
            children: [
              Icon(Icons.add, size: 18),
              SizedBox(width: 12),
              Text('New project…'),
            ],
          ),
        ),
      ],
      // 48 high because that is the smallest tap target the accessibility
      // guidelines allow, and the row of text and a caret is 28.
      child: ConstrainedBox(
        constraints: const BoxConstraints(minHeight: 48),
        child: Row(
          mainAxisSize: MainAxisSize.min,
          children: [
            Flexible(child: Text(current, overflow: TextOverflow.ellipsis)),
            const Icon(Icons.arrow_drop_down),
          ],
        ),
      ),
    );
  }

  /// Asks for a name. A new project is nothing but a name: the first note
  /// filed under it is what brings it into existence.
  Future<String?> _askForName(BuildContext context) {
    final controller = TextEditingController();
    return showDialog<String>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('New project'),
        content: TextField(
          controller: controller,
          autofocus: true,
          decoration: const InputDecoration(
            labelText: 'Name',
            helperText: 'It exists once a note is filed under it',
            border: OutlineInputBorder(),
          ),
          onSubmitted: (value) => Navigator.of(context).pop(value.trim()),
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(controller.text.trim()),
            child: const Text('Switch to it'),
          ),
        ],
      ),
    );
  }
}

/// The menu value that stands for "make one", rather than for a project.
const _newProject = '\u0000new';
