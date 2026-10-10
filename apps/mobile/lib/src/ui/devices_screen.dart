import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/tokens/device_token.dart';
import 'package:evomem_mobile/src/tokens/token_providers.dart';
import 'package:evomem_mobile/src/ui/clusters_screen.dart'
    show explainClusterProblem;
import 'package:evomem_mobile/src/ui/waiting_on_server.dart';
import 'package:flutter/material.dart';
import 'package:flutter/services.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Which devices can reach the server, and how to stop one (ADR-0030).
///
/// Evomem has no accounts and no login: this is one person's memory on their
/// own machine. What this screen manages is the unit of revocation — losing
/// a phone should cost a phone, not everything.
class DevicesScreen extends ConsumerStatefulWidget {
  /// Creates the screen.
  const new({super.key});

  @override
  ConsumerState<DevicesScreen> createState() => _DevicesScreenState();
}

class _DevicesScreenState extends ConsumerState<DevicesScreen> {
  final _name = TextEditingController();
  bool _busy = false;

  @override
  void dispose() {
    _name.dispose();
    super.dispose();
  }

  void _say(String message) {
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  void _report(ClusterFailure failure) {
    _say(
      failure.detail.isEmpty
          ? explainClusterProblem(failure.problem)
          : failure.detail,
    );
  }

  Future<void> _create() async {
    final name = _name.text.trim();
    if (name.isEmpty) {
      _say('Give it a name, so you can tell it from the others.');
      return;
    }

    setState(() => _busy = true);
    try {
      final service = await ref.read(tokenServiceProvider.future);
      final secret = await service.create(name);
      if (!mounted) return;
      _name.clear();
      ref.invalidate(deviceTokensProvider);
      await _showSecret(name, secret);
    } on ClusterFailure catch (failure) {
      if (!mounted) return;
      _report(failure);
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  /// Shows the secret, once.
  ///
  /// A dialog rather than a line in the list, because this is the only
  /// moment it exists: the server keeps a hash and nothing can read it
  /// back. Dismissing it loses it, and the dialog says so.
  Future<void> _showSecret(String name, String secret) {
    return showDialog<void>(
      context: context,
      barrierDismissible: false,
      builder: (context) => AlertDialog(
        title: Text('Token for $name'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          crossAxisAlignment: CrossAxisAlignment.start,
          children: [
            SelectableText(
              secret,
              style: const TextStyle(fontFamily: 'monospace'),
            ),
            const SizedBox(height: 12),
            const Text(
              'This is the only time it is shown. The server keeps a hash, '
              'so nothing can read it back — if it is lost, revoke it and '
              'make another.',
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () async {
              await Clipboard.setData(ClipboardData(text: secret));
              if (context.mounted) Navigator.of(context).pop();
            },
            child: const Text('Copy and close'),
          ),
          TextButton(
            onPressed: () => Navigator.of(context).pop(),
            child: const Text('Close'),
          ),
        ],
      ),
    );
  }

  Future<void> _revoke(DeviceToken token) async {
    try {
      final service = await ref.read(tokenServiceProvider.future);
      await service.revoke(token.id);
      if (!mounted) return;
      _say('${token.name} is refused from its next request');
      ref.invalidate(deviceTokensProvider);
    } on ClusterFailure catch (failure) {
      if (!mounted) return;
      _report(failure);
    }
  }

  @override
  Widget build(BuildContext context) {
    final tokens = ref.watch(deviceTokensProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Devices'),
        actions: [
          IconButton(
            tooltip: 'Reload',
            icon: const Icon(Icons.refresh),
            onPressed: () => ref.invalidate(deviceTokensProvider),
          ),
        ],
      ),
      body: tokens.when(
        loading: () => const WaitingOnServer(),
        error: (error, _) => Center(
          child: Padding(
            padding: const EdgeInsets.all(24),
            child: Text(
              error is ClusterFailure
                  ? (error.detail.isEmpty
                        ? explainClusterProblem(error.problem)
                        : error.detail)
                  : 'Could not load the devices.',
              textAlign: TextAlign.center,
            ),
          ),
        ),
        data: (list) => ListView(
          children: [
            const Padding(
              padding: EdgeInsets.all(16),
              child: Text(
                'Evomem has no accounts. A token is how a device says it is '
                'yours, and having one each is what makes losing a phone '
                'cost a phone.',
              ),
            ),
            for (final token in list)
              _TokenRow(token: token, onRevoke: () => _revoke(token)),
            const Divider(height: 32),
            Padding(
              padding: const EdgeInsets.symmetric(horizontal: 16),
              child: Column(
                crossAxisAlignment: CrossAxisAlignment.stretch,
                children: [
                  Text(
                    'Add a device',
                    style: Theme.of(context).textTheme.titleMedium,
                  ),
                  const SizedBox(height: 12),
                  TextField(
                    controller: _name,
                    decoration: const InputDecoration(
                      labelText: 'Name',
                      hintText: 'phone, browser, shortcut',
                      border: OutlineInputBorder(),
                    ),
                    onSubmitted: (_) => _busy ? null : _create(),
                  ),
                  const SizedBox(height: 16),
                  FilledButton(
                    onPressed: _busy ? null : _create,
                    child: Text(_busy ? 'Making…' : 'Make a token'),
                  ),
                  const SizedBox(height: 24),
                ],
              ),
            ),
          ],
        ),
      ),
    );
  }
}

class _TokenRow extends StatelessWidget {
  const new({required this.token, required this.onRevoke});

  final DeviceToken token;
  final VoidCallback onRevoke;

  @override
  Widget build(BuildContext context) {
    final lines = <String>[
      if (token.prefix.isNotEmpty) 'evm_${token.prefix}_…',
      // What makes revoking a decision rather than a guess.
      if (token.isRevoked)
        'revoked ${_day(token.revokedAt!)}'
      else if (token.lastUsedAt != null)
        'last used ${_day(token.lastUsedAt!)}'
      else
        'never used',
    ];

    return ListTile(
      title: Text(
        token.name,
        style: token.isRevoked
            ? TextStyle(
                decoration: TextDecoration.lineThrough,
                color: Theme.of(context).colorScheme.onSurfaceVariant,
              )
            : null,
      ),
      subtitle: Text(lines.join(' · ')),
      trailing: token.isRevoked
          ? null
          : IconButton(
              tooltip: 'Revoke',
              icon: const Icon(Icons.block),
              onPressed: onRevoke,
            ),
    );
  }

  static String _day(DateTime at) =>
      '${at.year}-${at.month.toString().padLeft(2, '0')}-'
      '${at.day.toString().padLeft(2, '0')}';
}
