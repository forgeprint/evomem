import 'dart:async';

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
                // Only an empty one can be forgotten, and only the name is:
                // a project with notes under it is known from the notes and
                // would come straight back, so offering it there would be a
                // button that does nothing.
                // Nor for the project the app starts in: it is always
                // offered, so forgetting it would be a button that appears
                // to do nothing.
                if (project.notes == 0 &&
                    project.projectId != current &&
                    project.projectId != defaultProjectId)
                  IconButton(
                    icon: const Icon(Icons.close, size: 16),
                    tooltip: 'Forget this name',
                    visualDensity: VisualDensity.compact,
                    onPressed: () {
                      Navigator.of(context).pop();
                      unawaited(
                        ref
                            .read(projectNamesRevisionProvider.notifier)
                            .forget(project.projectId),
                      );
                    },
                  )
                else
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

  /// Asks for a name.
  ///
  /// A project is still nothing but a name until a note is filed under it.
  /// The difference is that the name is now kept on this device, so one that
  /// has just been typed does not vanish the moment somebody looks at
  /// another project first.
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
            // Short enough to fit: the old sentence was clipped to
            // "It exists once a note is filed un…", so the one line that
            // explained the behaviour never reached anybody.
            helperText: 'Kept here until you write in it',
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
