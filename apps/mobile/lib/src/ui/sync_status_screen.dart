import 'dart:async';

import 'package:evomem_mobile/l10n/app_localizations.dart';
import 'package:evomem_mobile/src/routing/routes.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:evomem_mobile/src/storage/database.dart';
import 'package:evomem_mobile/src/sync/sync_service.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

/// Sync status screen showing sync state and manual sync trigger.
class SyncStatusScreen extends ConsumerStatefulWidget {
  /// Creates the screen.
  const new({super.key});

  @override
  ConsumerState<SyncStatusScreen> createState() => _SyncStatusScreenState();
}

class _SyncStatusScreenState extends ConsumerState<SyncStatusScreen> {
  bool _isSyncing = false;
  String? _lastSynced;
  String? _lastError;
  int _pendingNotes = 0;
  int _pendingDeletions = 0;

  @override
  void initState() {
    super.initState();
    // initState cannot await; the screen renders and fills in when the
    // read returns.
    unawaited(_loadSyncStatus());
  }

  Future<void> _loadSyncStatus() async {
    final dbHelper = ref.read(databaseHelperProvider);
    ref.read(syncServiceProvider);

    // Get last synced time from cursor
    final cursor = await _getCursor(dbHelper, 'sync_cursor_notes');

    if (cursor != null) {
      final parts = cursor.split('|');
      if (parts.length == 2) {
        _lastSynced = parts[0].replaceAll('T', ' ');
      }
    }

    // TODO(evomem): count what is actually pending from the cursor.
    // Until then this reads zero rather than a real figure.
    _pendingNotes = 0;
    _pendingDeletions = 0;

    if (mounted) setState(() {});
  }

  Future<String?> _getCursor(DatabaseHelper dbHelper, String key) async {
    final db = await dbHelper.database;
    final result = await db.query(
      'sync_state',
      where: 'key = ?',
      whereArgs: [key],
      limit: 1,
    );
    if (result.isEmpty) return null;
    return result.first['value'] as String?;
  }

  Future<void> _syncNow() async {
    final syncService = ref.read(syncServiceProvider.notifier);

    if (!syncService.isInitialized) {
      setState(() {
        _lastError =
            'Sync not configured. Set server URL and API token in Settings.';
      });
      return;
    }

    setState(() {
      _isSyncing = true;
      _lastError = null;
    });

    try {
      final result = await syncService.push();

      if (mounted) {
        setState(() {
          _isSyncing = false;
          if (result.success) {
            _lastSynced = DateTime.now()
                .toString()
                .substring(0, 16)
                .replaceAll('T', ' ');
            _pendingNotes = 0;
            _pendingDeletions = 0;
            _lastError = null;
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(content: Text(AppLocalizations.of(context).noteSaved)),
            );
          } else {
            _lastError = result.error;
            ScaffoldMessenger.of(context).showSnackBar(
              SnackBar(
                content: Text(
                  AppLocalizations.of(context)
                      .syncFailed(result.error ?? 'Unknown error'),
                ),
              ),
            );
          }
        });
      }
    } on Exception catch (e) {
      if (mounted) {
        setState(() {
          _isSyncing = false;
          _lastError = e.toString();
        });
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text(
              AppLocalizations.of(context).syncFailed(e.toString()),
            ),
          ),
        );
      }
    }
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.syncStatus),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => context.go(const NotesListRoute().location),
        ),
      ),
      body: Padding(
        padding: const EdgeInsets.all(16),
        child: Column(
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Card(
              key: const Key('sync_status_card'),
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Sync Status',
                      key: const Key('sync_status_title'),
                      style: Theme.of(context).textTheme.titleLarge,
                    ),
                    const SizedBox(height: 16),
                    _StatusRow(
                      label: 'Last synced',
                      value: _lastSynced != null
                          ? l10n.lastSynced(_lastSynced!)
                          : 'Never',
                    ),
                    const SizedBox(height: 8),
                    _StatusRow(
                      label: 'Pending Changes',
                      value:
                          '$_pendingNotes notes, $_pendingDeletions deletions',
                    ),
                    const SizedBox(height: 8),
                    _StatusRow(
                      label: 'Status',
                      value: _isSyncing
                          ? l10n.syncing
                          : (_lastError != null ? 'Error' : 'Idle'),
                      valueStyle: _isSyncing
                          ? TextStyle(
                              color: Theme.of(context).colorScheme.primary,
                            )
                          : _lastError != null
                          ? const TextStyle(color: Colors.red)
                          : null,
                    ),
                    if (_lastError != null) ...[
                      const SizedBox(height: 8),
                      Text(
                        _lastError!,
                        style: const TextStyle(color: Colors.red, fontSize: 12),
                      ),
                    ],
                  ],
                ),
              ),
            ),
            const SizedBox(height: 24),
            SizedBox(
              width: double.infinity,
              child: ElevatedButton.icon(
                onPressed: _isSyncing ? null : _syncNow,
                icon: _isSyncing
                    ? const SizedBox(
                        width: 20,
                        height: 20,
                        child: CircularProgressIndicator(strokeWidth: 2),
                      )
                    : const Icon(Icons.sync),
                label: Text(_isSyncing ? l10n.syncing : l10n.syncNow),
                style: ElevatedButton.styleFrom(
                  padding: const EdgeInsets.symmetric(vertical: 16),
                ),
              ),
            ),
            const SizedBox(height: 24),
            Card(
              child: Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      'Sync Configuration',
                      style: Theme.of(context).textTheme.titleLarge,
                    ),
                    const SizedBox(height: 16),
                    const Text(
                      'Configure the Evomem server URL and API token in '
                      'Settings to enable cloud synchronization.',
                      style: TextStyle(color: Colors.grey),
                    ),
                    const SizedBox(height: 16),
                    Text(
                      'The sync is one-directional: local SQLite to remote '
                      'PostgreSQL. Local data is the authority; the cloud is '
                      'a mirror. Archived notes (older than 6 months) are '
                      'thinned locally but kept in the cloud.',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _StatusRow extends StatelessWidget {
  const new({required this.label, required this.value, this.valueStyle});

  final String label;
  final String value;
  final TextStyle? valueStyle;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(label, style: Theme.of(context).textTheme.bodyMedium),
        Text(
          value,
          style:
              valueStyle ??
              Theme.of(context).textTheme.bodyMedium
                  ?.copyWith(fontWeight: FontWeight.w500),
        ),
      ],
    );
  }
}
