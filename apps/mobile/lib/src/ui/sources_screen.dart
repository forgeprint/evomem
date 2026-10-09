import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/sources/source_connection.dart';
import 'package:evomem_mobile/src/sources/source_providers.dart';
import 'package:evomem_mobile/src/ui/clusters_screen.dart'
    show explainClusterProblem;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// The sources the server pulls from, and the button that pulls.
///
/// The panel (ADR-0026). A source somebody else owns is pulled by the server
/// calling their API; this is where a person says which sources, with which
/// credentials, and presses sync.
class SourcesScreen extends ConsumerStatefulWidget {
  /// Creates the screen.
  const new({super.key});

  @override
  ConsumerState<SourcesScreen> createState() => _SourcesScreenState();
}

class _SourcesScreenState extends ConsumerState<SourcesScreen> {
  bool _pulling = false;

  Future<void> _pull() async {
    setState(() => _pulling = true);
    try {
      final service = await ref.read(sourceServiceProvider.future);
      final outcome = await service.pull();
      if (!mounted) return;
      _say(
        '${outcome.pulled} note(s) from ${outcome.considered} source(s)'
        '${outcome.failed > 0 ? ', ${outcome.failed} failed' : ''}',
      );
      ref.invalidate(sourceConnectionsProvider);
    } on ClusterFailure catch (failure) {
      if (!mounted) return;
      _say(
        failure.detail.isEmpty
            ? explainClusterProblem(failure.problem)
            : failure.detail,
      );
    } finally {
      if (mounted) setState(() => _pulling = false);
    }
  }

  Future<void> _forget(SourceConnection connection) async {
    try {
      final service = await ref.read(sourceServiceProvider.future);
      await service.forget(connection.id);
      if (!mounted) return;
      _say('forgot ${connection.sourceType}, and its token with it');
      ref.invalidate(sourceConnectionsProvider);
    } on ClusterFailure catch (failure) {
      if (!mounted) return;
      _say(
        failure.detail.isEmpty
            ? explainClusterProblem(failure.problem)
            : failure.detail,
      );
    }
  }

  void _say(String message) {
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  @override
  Widget build(BuildContext context) {
    final connections = ref.watch(sourceConnectionsProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Sources'),
        actions: [
          IconButton(
            tooltip: 'Reload',
            icon: const Icon(Icons.refresh),
            onPressed: () => ref.invalidate(sourceConnectionsProvider),
          ),
        ],
      ),
      floatingActionButton: FloatingActionButton.extended(
        onPressed: _pulling ? null : _pull,
        icon: _pulling
            ? const SizedBox(
                width: 18,
                height: 18,
                child: CircularProgressIndicator(strokeWidth: 2),
              )
            : const Icon(Icons.sync),
        label: Text(_pulling ? 'Pulling…' : 'Sync now'),
      ),
      body: connections.when(
        loading: () => const Center(child: CircularProgressIndicator()),
        error: (error, _) => Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text(
              error is ClusterFailure
                  ? (error.detail.isEmpty
                        ? explainClusterProblem(error.problem)
                        : error.detail)
                  : 'Could not load the sources.',
              textAlign: TextAlign.center,
            ),
          ),
        ),
        data: (list) => ListView(
          padding: const EdgeInsets.only(bottom: 88),
          children: [
            if (list.isEmpty)
              const Padding(
                padding: EdgeInsets.all(24),
                child: Text(
                  'No sources connected. A source somebody else owns — Jira '
                  'today — is pulled by this server calling their API.',
                  textAlign: TextAlign.center,
                ),
              ),
            for (final connection in list)
              _ConnectionRow(
                connection: connection,
                onForget: () => _forget(connection),
              ),
            const Divider(height: 32),
            _ConnectForm(
              onConnected: () => ref.invalidate(sourceConnectionsProvider),
              say: _say,
            ),
          ],
        ),
      ),
    );
  }
}

class _ConnectionRow extends StatelessWidget {
  const new({required this.connection, required this.onForget});

  final SourceConnection connection;
  final VoidCallback onForget;

  @override
  Widget build(BuildContext context) {
    final lines = <String>[
      connection.baseUrl,
      if (connection.account.isNotEmpty) 'as ${connection.account}',
      if (connection.query.isNotEmpty) 'asks for: ${connection.query}',
      if (connection.lastPulledAt != null)
        'last pulled ${connection.lastPulledAt}',
      // The failure is shown, not buried: a stale source is otherwise a
      // mystery.
      if (connection.lastError.isNotEmpty)
        'last failure: ${connection.lastError}',
    ];

    return ListTile(
      title: Text('${connection.sourceType} → ${connection.projectId}'),
      subtitle: Text(lines.join('\n')),
      isThreeLine: true,
      trailing: IconButton(
        tooltip: 'Forget this source',
        icon: const Icon(Icons.delete_outline),
        onPressed: onForget,
      ),
    );
  }
}

/// Connects a source. The token is typed here, sent once, and never read
/// back: the server does not return it (ADR-0026).
class _ConnectForm extends ConsumerStatefulWidget {
  const new({required this.onConnected, required this.say});

  final VoidCallback onConnected;
  final void Function(String) say;

  @override
  ConsumerState<_ConnectForm> createState() => _ConnectFormState();
}

class _ConnectFormState extends ConsumerState<_ConnectForm> {
  final _url = TextEditingController();
  final _account = TextEditingController();
  final _query = TextEditingController();
  final _secret = TextEditingController();
  final _project = TextEditingController(text: 'default');
  bool _busy = false;

  @override
  void dispose() {
    for (final controller in [_url, _account, _query, _secret, _project]) {
      controller.dispose();
    }
    super.dispose();
  }

  Future<void> _connect() async {
    setState(() => _busy = true);
    try {
      final service = await ref.read(sourceServiceProvider.future);
      await service.connect(
        sourceType: 'jira',
        projectId: _project.text.trim(),
        baseUrl: _url.text.trim(),
        account: _account.text.trim(),
        query: _query.text.trim(),
        secret: _secret.text,
      );
      if (!mounted) return;
      // Cleared as soon as it has been sent: a token sitting in a field is a
      // token sitting on a screen.
      _secret.clear();
      widget.say('connected');
      widget.onConnected();
    } on ClusterFailure catch (failure) {
      if (!mounted) return;
      widget.say(
        failure.detail.isEmpty
            ? explainClusterProblem(failure.problem)
            : failure.detail,
      );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  @override
  Widget build(BuildContext context) {
    return Padding(
      padding: const EdgeInsets.symmetric(horizontal: 16),
      child: Column(
        crossAxisAlignment: CrossAxisAlignment.stretch,
        children: [
          Text('Connect Jira', style: Theme.of(context).textTheme.titleMedium),
          const SizedBox(height: 12),
          TextField(
            controller: _url,
            decoration: const InputDecoration(
              labelText: 'Site address',
              hintText: 'https://you.atlassian.net',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _account,
            decoration: const InputDecoration(
              labelText: 'Account email',
              helperText: 'Jira takes the email, not the username',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _secret,
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'API token',
              helperText:
                  'Sent once and sealed on the server; never shown again',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _query,
            decoration: const InputDecoration(
              labelText: 'JQL',
              hintText: 'project = EVO ORDER BY updated DESC',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _project,
            decoration: const InputDecoration(
              labelText: 'File notes under project',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 16),
          FilledButton(
            onPressed: _busy ? null : _connect,
            child: Text(_busy ? 'Connecting…' : 'Connect'),
          ),
        ],
      ),
    );
  }
}
