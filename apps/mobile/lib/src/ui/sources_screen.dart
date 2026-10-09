import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/sources/source_connection.dart';
import 'package:evomem_mobile/src/sources/source_providers.dart';
import 'package:evomem_mobile/src/state/notes_notifier.dart';
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
  bool _organizing = false;

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

  Future<void> _organize() async {
    setState(() => _organizing = true);
    try {
      final service = await ref.read(sourceServiceProvider.future);
      // The project on screen, not the server's default: grouping the notes
      // of a project nobody is looking at is a surprise.
      final outcome = await service.organize(
        projectId: ref.read(currentProjectProvider),
      );
      if (!mounted) return;
      final dropped = outcome.invented > 0
          ? ', ${outcome.invented} id(s) dropped'
          : '';
      _say(
        outcome.offered == 0
            ? 'every note is already in a group'
            : '${outcome.created} group(s) over '
                  '${outcome.grouped} note(s)$dropped',
      );
    } on ClusterFailure catch (failure) {
      if (!mounted) return;
      _say(
        failure.detail.isEmpty
            ? explainClusterProblem(failure.problem)
            : failure.detail,
      );
    } finally {
      if (mounted) setState(() => _organizing = false);
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

  /// Whether a model is connected, which is what makes grouping offerable.
  bool _hasModel(AsyncValue<List<SourceConnection>> connections) {
    final list = connections.asData?.value;
    return list != null && list.any((c) => c.isModel);
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
      floatingActionButton: Column(
        mainAxisSize: MainAxisSize.min,
        crossAxisAlignment: CrossAxisAlignment.end,
        children: [
          // Only offered when a model is connected: a button that can only
          // answer "nothing is configured" is a button that teaches nothing.
          if (_hasModel(connections)) ...[
            FloatingActionButton.extended(
              heroTag: 'organize',
              onPressed: _organizing ? null : _organize,
              icon: _organizing
                  ? const _Spinner()
                  : const Icon(Icons.auto_awesome_motion),
              label: Text(_organizing ? 'Grouping…' : 'Group notes'),
            ),
            const SizedBox(height: 12),
          ],
          FloatingActionButton.extended(
            heroTag: 'pull',
            onPressed: _pulling ? null : _pull,
            icon: _pulling ? const _Spinner() : const Icon(Icons.sync),
            label: Text(_pulling ? 'Pulling…' : 'Sync now'),
          ),
        ],
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
            const Divider(height: 32),
            _ModelForm(
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
      // A model row is in the same keyring but is not a source: it is never
      // pulled from, and "asks for" would read as a search (ADR-0027).
      if (connection.isModel)
        'groups notes with: ${connection.modelName}'
      else if (connection.query.isNotEmpty)
        'asks for: ${connection.query}',
      if (connection.lastPulledAt != null)
        'last pulled ${connection.lastPulledAt}',
      // The failure is shown, not buried: a stale source is otherwise a
      // mystery.
      if (connection.lastError.isNotEmpty)
        'last failure: ${connection.lastError}',
    ];

    return ListTile(
      title: Text(
        connection.isModel
            ? 'model → ${connection.projectId}'
            : '${connection.sourceType} → ${connection.projectId}',
      ),
      subtitle: Text(lines.join('\n')),
      isThreeLine: true,
      trailing: IconButton(
        tooltip: connection.isModel
            ? 'Forget this model key'
            : 'Forget this source',
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

/// The spinner a busy button shows in place of its icon.
class _Spinner extends StatelessWidget {
  const new();

  @override
  Widget build(BuildContext context) => const SizedBox(
    width: 18,
    height: 18,
    child: CircularProgressIndicator(strokeWidth: 2),
  );
}

/// Connects the model that groups notes on the server.
///
/// The other half of "panelden girdiğim yapay zeka API'si" (ADR-0027). The
/// key is typed here, sent once and sealed on the server; the panel never
/// reads it back.
class _ModelForm extends ConsumerStatefulWidget {
  const new({required this.onConnected, required this.say});

  final VoidCallback onConnected;
  final void Function(String) say;

  @override
  ConsumerState<_ModelForm> createState() => _ModelFormState();
}

class _ModelFormState extends ConsumerState<_ModelForm> {
  final _url = TextEditingController(text: 'https://api.openai.com');
  final _model = TextEditingController(text: 'gpt-4o-mini');
  final _secret = TextEditingController();
  final _project = TextEditingController(text: 'default');
  bool _busy = false;

  @override
  void dispose() {
    for (final controller in [_url, _model, _secret, _project]) {
      controller.dispose();
    }
    super.dispose();
  }

  Future<void> _connect() async {
    setState(() => _busy = true);
    try {
      final service = await ref.read(sourceServiceProvider.future);
      await service.connect(
        sourceType: modelSourceType,
        projectId: _project.text.trim(),
        baseUrl: _url.text.trim(),
        // The server keeps which model in the query column, which for a
        // model means which model.
        query: _model.text.trim(),
        secret: _secret.text,
      );
      if (!mounted) return;
      _secret.clear();
      widget.say('model connected');
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
          Text(
            'Connect a model',
            style: Theme.of(context).textTheme.titleMedium,
          ),
          const SizedBox(height: 8),
          // Said at the point the key is entered, not buried in a record
          // nobody opens: this is the moment a person decides that their
          // notes may leave the machine (ADR-0027).
          Text(
            'With a model connected, pressing "Group notes" sends the text '
            'of the notes being grouped to this address. An agent connected '
            'over MCP can group notes without any of this.',
            style: Theme.of(context).textTheme.bodySmall,
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _url,
            decoration: const InputDecoration(
              labelText: 'Server address',
              helperText: 'Any OpenAI-compatible server, including a local one',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _model,
            decoration: const InputDecoration(
              labelText: 'Model',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _secret,
            obscureText: true,
            decoration: const InputDecoration(
              labelText: 'API key',
              helperText:
                  'Sent once and sealed on the server; never shown again',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 12),
          TextField(
            controller: _project,
            decoration: const InputDecoration(
              labelText: 'Group notes in project',
              border: OutlineInputBorder(),
            ),
          ),
          const SizedBox(height: 16),
          FilledButton(
            onPressed: _busy ? null : _connect,
            child: Text(_busy ? 'Connecting…' : 'Connect model'),
          ),
        ],
      ),
    );
  }
}
