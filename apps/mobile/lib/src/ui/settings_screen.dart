import 'dart:async';

import 'package:evomem_mobile/l10n/app_localizations.dart';
import 'package:evomem_mobile/src/routing/routes.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';
import 'package:evomem_mobile/src/sync/sync_service.dart';
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';
import 'package:go_router/go_router.dart';
import 'package:http/http.dart' as http;

/// Settings screen for the app.
class SettingsScreen extends ConsumerStatefulWidget {
  /// Creates the screen.
  const new({super.key});

  @override
  ConsumerState<SettingsScreen> createState() => _SettingsScreenState();
}

class _SettingsScreenState extends ConsumerState<SettingsScreen> {
  final _storage = const FlutterSecureStorage();
  final _serverUrlController = TextEditingController();
  final _apiTokenController = TextEditingController();
  String? _serverUrl;
  String? _apiToken;
  bool _testingConnection = false;

  @override
  void initState() {
    super.initState();
    unawaited(_loadSettings());
  }

  Future<void> _loadSettings() async {
    _serverUrl = await _storage.read(key: 'evomem_server_url');
    _apiToken = await _storage.read(key: 'evomem_api_token');
    _serverUrlController.text = _serverUrl ?? '';
    _apiTokenController.text = _apiToken ?? '';
    if (mounted) setState(() {});
  }

  Future<void> _saveSettings() async {
    await _storage.write(
      key: 'evomem_server_url',
      value: _serverUrlController.text,
    );
    await _storage.write(
      key: 'evomem_api_token',
      value: _apiTokenController.text,
    );
    _serverUrl = _serverUrlController.text;
    _apiToken = _apiTokenController.text;

    // Initialize sync service with new config
    if (_serverUrl != null &&
        _serverUrl!.isNotEmpty &&
        _apiToken != null &&
        _apiToken!.isNotEmpty) {
      ref
          .read(syncServiceProvider.notifier)
          .initialize(
            serverUrl: _serverUrl!,
            apiToken: _apiToken!,
            notesDao: ref.read(notesDaoProvider),
            dbHelper: ref.read(databaseHelperProvider),
          );
    }

    if (mounted) {
      ScaffoldMessenger.of(context)
          .showSnackBar(const SnackBar(content: Text('Settings saved')));
    }
  }

  Future<void> _testConnection() async {
    if (_serverUrlController.text.isEmpty || _apiTokenController.text.isEmpty) {
      ScaffoldMessenger.of(context).showSnackBar(
        const SnackBar(
          content: Text('Please enter server URL and API token first'),
        ),
      );
      return;
    }

    setState(() => _testingConnection = true);

    try {
      final client = http.Client();
      final uri = Uri.parse('${_serverUrlController.text}/healthz');
      final headers = {'Authorization': 'Bearer ${_apiTokenController.text}'};

      final response = await client
          .get(uri, headers: headers)
          .timeout(const Duration(seconds: 10));
      client.close();

      if (mounted) {
        if (response.statusCode == 200) {
          ScaffoldMessenger.of(context).showSnackBar(
            const SnackBar(
              content: Text('Connection successful!'),
              backgroundColor: Colors.green,
            ),
          );
        } else {
          ScaffoldMessenger.of(context).showSnackBar(
            SnackBar(
              content: Text('Connection failed: ${response.statusCode}'),
              backgroundColor: Colors.red,
            ),
          );
        }
      }
    } on Exception catch (e) {
      if (mounted) {
        ScaffoldMessenger.of(context).showSnackBar(
          SnackBar(
            content: Text('Connection error: $e'),
            backgroundColor: Colors.red,
          ),
        );
      }
    } finally {
      if (mounted) setState(() => _testingConnection = false);
    }
  }

  @override
  void dispose() {
    _serverUrlController.dispose();
    _apiTokenController.dispose();
    super.dispose();
  }

  @override
  Widget build(BuildContext context) {
    final l10n = AppLocalizations.of(context);

    return Scaffold(
      appBar: AppBar(
        title: Text(l10n.settings),
        leading: IconButton(
          icon: const Icon(Icons.arrow_back),
          onPressed: () => context.go(const NotesListRoute().location),
        ),
      ),
      body: ListView(
        padding: const EdgeInsets.all(16),
        children: [
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'API Configuration',
                    style: Theme.of(context).textTheme.titleLarge,
                  ),
                  const SizedBox(height: 16),
                  TextField(
                    controller: _serverUrlController,
                    decoration: const InputDecoration(
                      labelText: 'Server URL (e.g., http://192.168.1.100:8765)',
                      border: OutlineInputBorder(),
                      hintText: 'http://localhost:8765',
                    ),
                  ),
                  const SizedBox(height: 16),
                  TextField(
                    controller: _apiTokenController,
                    decoration: const InputDecoration(
                      labelText: 'API Token (Bearer)',
                      border: OutlineInputBorder(),
                      hintText: 'Enter your bearer token',
                    ),
                    obscureText: true,
                  ),
                  const SizedBox(height: 16),
                  Row(
                    children: [
                      Expanded(
                        child: ElevatedButton(
                          onPressed: _saveSettings,
                          child: const Text('Save Settings'),
                        ),
                      ),
                      const SizedBox(width: 12),
                      Expanded(
                        child: OutlinedButton.icon(
                          onPressed: _testingConnection
                              ? null
                              : _testConnection,
                          icon: _testingConnection
                              ? const SizedBox(
                                  width: 18,
                                  height: 18,
                                  child: CircularProgressIndicator(
                                    strokeWidth: 2,
                                  ),
                                )
                              : const Icon(Icons.cloud),
                          label: Text(
                            _testingConnection
                                ? 'Testing...'
                                : 'Test Connection',
                          ),
                        ),
                      ),
                    ],
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),
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
                  Consumer(
                    builder: (context, ref, _) {
                      final syncService = ref.watch(syncServiceProvider);
                      final initialized = syncService.isInitialized;
                      final config = syncService.config;

                      return Column(
                        crossAxisAlignment: CrossAxisAlignment.start,
                        children: [
                          _ConfigRow(
                            label: 'Status',
                            value: initialized
                                ? 'Configured'
                                : 'Not configured',
                            valueStyle: initialized
                                ? const TextStyle(
                                    color: Colors.green,
                                    fontWeight: FontWeight.w600,
                                  )
                                : const TextStyle(
                                    color: Colors.orange,
                                    fontWeight: FontWeight.w600,
                                  ),
                          ),
                          if (initialized && config != null) ...[
                            const SizedBox(height: 8),
                            _ConfigRow(
                              label: 'Server',
                              value: config.serverUrl,
                            ),
                            const SizedBox(height: 8),
                            _ConfigRow(
                              label: 'Batch Size',
                              value: '${config.batchSize} notes',
                            ),
                            const SizedBox(height: 8),
                            _ConfigRow(
                              label: 'Timeout',
                              value: '${config.timeout.inSeconds}s',
                            ),
                          ],
                        ],
                      );
                    },
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text(
                    'Local Storage',
                    style: Theme.of(context).textTheme.titleLarge,
                  ),
                  const SizedBox(height: 16),
                  ListTile(
                    leading: const Icon(Icons.storage),
                    title: const Text('Database Path'),
                    subtitle: Text(_getDatabasePath()),
                    trailing: const Icon(Icons.chevron_right),
                    onTap: () => unawaited(_showDatabaseInfo()),
                  ),
                  ListTile(
                    leading: const Icon(
                      Icons.delete_outline,
                      color: Colors.red,
                    ),
                    title: const Text(
                      'Clear All Local Data',
                      style: TextStyle(color: Colors.red),
                    ),
                    onTap: () => unawaited(_confirmClearData()),
                  ),
                ],
              ),
            ),
          ),
          const SizedBox(height: 16),
          Card(
            child: Padding(
              padding: const EdgeInsets.all(16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.start,
                children: [
                  Text('About', style: Theme.of(context).textTheme.titleLarge),
                  const SizedBox(height: 16),
                  const ListTile(
                    leading: Icon(Icons.info_outline),
                    title: Text('Version'),
                    subtitle: Text('0.1.0'),
                  ),
                  const ListTile(
                    leading: Icon(Icons.code),
                    title: Text('Open Source'),
                    subtitle: Text('Apache-2.0 License'),
                  ),
                ],
              ),
            ),
          ),
        ],
      ),
    );
  }

  String _getDatabasePath() {
    return 'app_documents/evomem.db';
  }

  Future<void> _showDatabaseInfo() async {
    return await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Database Info'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            Text('Path: $_getDatabasePath()'),
            const SizedBox(height: 8),
            const Text('Type: SQLite (sqflite)'),
            const Text('Schema: Compatible with Evomem Go backend'),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('OK'),
          ),
        ],
      ),
    );
  }

  Future<void> _confirmClearData() async {
    final l10n = AppLocalizations.of(context);
    return await showDialog<void>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Clear All Data?'),
        content: const Text(
          'This will delete all local notes and settings. '
          'This cannot be undone.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: Text(l10n.cancel),
          ),
          TextButton(
            onPressed: () async {
              // Taken from the dialog's own context before the awaits. The
              // State's `mounted` says nothing about whether this dialog is
              // still up, and replaceAll has to finish before the dialog
              // reports the data as cleared.
              final navigator = Navigator.of(context);
              final messenger = ScaffoldMessenger.of(context);

              await _storage.deleteAll();
              await ref.read(notesProvider.notifier).replaceAll([]);
              ref.read(syncServiceProvider.notifier).dispose();

              navigator.pop();
              messenger.showSnackBar(
                const SnackBar(content: Text('All local data cleared')),
              );
            },
            child: const Text('Clear', style: TextStyle(color: Colors.red)),
          ),
        ],
      ),
    );
  }
}

class _ConfigRow extends StatelessWidget {
  const new({required this.label, required this.value, this.valueStyle});

  final String label;
  final String value;
  final TextStyle? valueStyle;

  @override
  Widget build(BuildContext context) {
    return Row(
      mainAxisAlignment: MainAxisAlignment.spaceBetween,
      children: [
        Text(
          label,
          style: Theme.of(context).textTheme.bodyMedium
              ?.copyWith(color: Colors.grey[600]),
        ),
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
