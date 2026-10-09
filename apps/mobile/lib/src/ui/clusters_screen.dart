import 'package:evomem_mobile/src/clusters/cluster.dart';
import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/routing/routes.dart';
import 'package:evomem_mobile/src/ui/waiting_on_server.dart';
import 'package:flutter/foundation.dart' show kIsWeb;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

/// What to tell somebody when there are no groupings to show.
///
/// A cluster lives on the server and nowhere else (ADR-0024), so an empty
/// screen has several meanings and they need different answers: nothing is
/// configured, nothing could be reached, or nothing has been grouped yet.
String explainClusterProblem(ClusterProblem problem) => switch (problem) {
  ClusterProblem.notConfigured => _notConfigured,
  ClusterProblem.unreachable =>
    'The server could not be reached. Groupings are not stored on this '
        'device, so there is nothing to show while it is away.',
  ClusterProblem.unreadable =>
    'The server answered something this version cannot read.',
};

/// In a browser the settings belong to the address they were saved under:
/// localhost and 127.0.0.1 are different origins with separate storage, so
/// the same app at the other one looks as though it was never configured.
/// That is a confusing enough half-hour to be worth a sentence.
const _sameAppOtherAddress =
    '\n\nIf you have set them already, check the address bar: settings saved '
    'at localhost are not visible at 127.0.0.1, or the other way round.';

const _notConfigured =
    'Set the server address and token in Settings. Groupings are made on '
    'the server, so this screen needs one.'
    '${kIsWeb ? _sameAppOtherAddress : ''}';

/// The groupings the server made over this project's notes.
class ClustersScreen extends ConsumerWidget {
  /// Creates the screen.
  const new({super.key});

  @override
  Widget build(BuildContext context, WidgetRef ref) {
    final clusters = ref.watch(clustersProvider);

    return Scaffold(
      appBar: AppBar(
        title: const Text('Groups'),
        actions: [
          IconButton(
            tooltip: 'Reload',
            icon: const Icon(Icons.refresh),
            onPressed: () => ref.invalidate(clustersProvider),
          ),
        ],
      ),
      body: clusters.when(
        loading: () => const WaitingOnServer(),
        error: (error, _) => _Explanation(
          message: error is ClusterFailure
              ? explainClusterProblem(error.problem)
              : 'Could not load the groupings.',
          detail: error is ClusterFailure ? error.detail : '',
          onRetry: () => ref.invalidate(clustersProvider),
        ),
        data: (list) => list.isEmpty
            ? const _Explanation(
                message:
                    'Nothing has been grouped yet. An agent connected over '
                    'MCP makes the groups; this screen shows what it did.',
              )
            : RefreshIndicator(
                onRefresh: () async => ref.invalidate(clustersProvider),
                child: ListView.builder(
                  itemCount: list.length,
                  itemBuilder: (context, index) =>
                      _ClusterRow(cluster: list[index]),
                ),
              ),
      ),
    );
  }
}

class _ClusterRow extends StatelessWidget {
  const new({required this.cluster});

  final Cluster cluster;

  @override
  Widget build(BuildContext context) {
    return ListTile(
      title: Text(cluster.name),
      subtitle: cluster.summary.isEmpty ? null : Text(cluster.summary),
      // The size is the part worth reading: a group of one and a group of
      // everything are both usually mistakes.
      leading: CircleAvatar(child: Text('${cluster.size}')),
      trailing: const Icon(Icons.chevron_right),
      onTap: () => context.go(ClusterDetailRoute(cluster.id).location),
    );
  }
}

class _Explanation extends StatelessWidget {
  const new({required this.message, this.detail = '', this.onRetry});

  final String message;
  final String detail;
  final VoidCallback? onRetry;

  @override
  Widget build(BuildContext context) {
    return Center(
      child: Padding(
        padding: const EdgeInsets.all(24),
        child: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            Text(message, textAlign: TextAlign.center),
            if (detail.isNotEmpty) ...[
              const SizedBox(height: 8),
              Text(
                detail,
                textAlign: TextAlign.center,
                style: Theme.of(context).textTheme.bodySmall,
              ),
            ],
            if (onRetry != null) ...[
              const SizedBox(height: 16),
              OutlinedButton(
                onPressed: onRetry,
                child: const Text('Try again'),
              ),
            ],
          ],
        ),
      ),
    );
  }
}
