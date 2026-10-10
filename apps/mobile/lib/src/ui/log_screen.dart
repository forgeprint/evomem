import 'package:evomem_mobile/src/log/log.dart';
import 'package:evomem_mobile/src/routing/routes.dart';
import 'package:evomem_mobile/src/ui/back_to.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// What the app has been doing, newest first.
///
/// It exists because the two places this app runs have no console a person
/// will look at: a browser hides it behind devtools and a phone has none.
/// Without this, a recording that fails says "it did not work" and the
/// reason goes nowhere.
class LogScreen extends ConsumerWidget {
  /// Creates the screen.
  const new({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final entries = ref.watch(logbookProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Log'),
        leading: BackTo(const NotesListRoute().location),
        actions: [
          IconButton(
            tooltip: 'Copy all',
            icon: const Icon(Icons.copy_all),
            onPressed: entries.isEmpty
                ? null
                : () async {
                    await Clipboard.setData(
                      ClipboardData(text: entries.join('\n')),
                    );
                    if (!context.mounted) return;
                    ScaffoldMessenger.of(context)
                        .showSnackBar(const SnackBar(content: Text('copied')));
                  },
          ),
          IconButton(
            tooltip: 'Clear',
            icon: const Icon(Icons.delete_outline),
            onPressed: entries.isEmpty
                ? null
                : () => ref.read(logbookProvider.notifier).clear(),
          ),
        ],
      ),
      body: entries.isEmpty
          ? const Center(
              child: Padding(
                padding: EdgeInsets.all(24),
                child: Text(
                  'Nothing yet. What the app does — a recording starting, a '
                  'sync failing — shows up here.',
                  textAlign: TextAlign.center,
                ),
              ),
            )
          : ListView.separated(
              itemCount: entries.length,
              separatorBuilder: (_, _) => const Divider(height: 1),
              itemBuilder: (context, i) => _EntryRow(entries[i]),
            ),
    );
  }
}

class _EntryRow extends StatelessWidget {
  const new(this.entry);

  final LogEntry entry;

  @override
  Widget build(BuildContext context) {
    final theme = Theme.of(context);
    final colour = switch (entry.level) {
      LogLevel.info => theme.colorScheme.onSurfaceVariant,
      LogLevel.warning => theme.colorScheme.tertiary,
      LogLevel.error => theme.colorScheme.error,
    };
    final at = entry.at.toIso8601String().substring(11, 19);

    return ListTile(
      dense: true,
      leading: Icon(
        switch (entry.level) {
          LogLevel.info => Icons.info_outline,
          LogLevel.warning => Icons.warning_amber_outlined,
          LogLevel.error => Icons.error_outline,
        },
        size: 18,
        color: colour,
      ),
      title: Text(entry.message, style: TextStyle(color: colour)),
      subtitle: Text(
        // The machine's own words, which is the part worth copying.
        entry.detail.isEmpty
            ? '$at · ${entry.source}'
            : '$at · ${entry.source}\n${entry.detail}',
        style: theme.textTheme.bodySmall,
      ),
      isThreeLine: entry.detail.isNotEmpty,
    );
  }
}
