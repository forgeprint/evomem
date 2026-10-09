import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/ui/clusters_screen.dart'
    show explainClusterProblem;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// One grouping and the notes in it.
///
/// Fetched by the screen rather than through a provider family: this is the
/// only consumer, and the id arrives from the route.
class ClusterDetailScreen extends ConsumerStatefulWidget {
  /// Creates the screen for the grouping with this [clusterId].
  const new({required this.clusterId, super.key});

  /// The grouping to show.
  final String clusterId;

  @override
  ConsumerState<ClusterDetailScreen> createState() =>
      _ClusterDetailScreenState();
}

class _ClusterDetailScreenState extends ConsumerState<ClusterDetailScreen> {
  late Future<ClusterDetail> _detail;

  @override
  void initState() {
    super.initState();
    _detail = _load();
  }

  Future<ClusterDetail> _load() async {
    final service = await ref.read(clusterServiceProvider.future);
    return await service.get(widget.clusterId);
  }

  @override
  Widget build(BuildContext context) {
    return Scaffold(
      appBar: AppBar(
        title: const Text('Group'),
        actions: [
          IconButton(
            tooltip: 'Reload',
            icon: const Icon(Icons.refresh),
            onPressed: () => setState(() => _detail = _load()),
          ),
        ],
      ),
      body: FutureBuilder<ClusterDetail>(
        future: _detail,
        builder: (context, snapshot) {
          if (snapshot.connectionState != ConnectionState.done) {
            return const Center(child: CircularProgressIndicator());
          }
          final error = snapshot.error;
          if (error != null) {
            return Center(
              child: Padding(
                padding: const EdgeInsets.all(24),
                child: Text(
                  error is ClusterFailure
                      ? explainClusterProblem(error.problem)
                      : 'Could not load this group.',
                  textAlign: TextAlign.center,
                ),
              ),
            );
          }

          final detail = snapshot.data!;
          return ListView(
            children: [
              Padding(
                padding: const EdgeInsets.all(16),
                child: Column(
                  crossAxisAlignment: CrossAxisAlignment.start,
                  children: [
                    Text(
                      detail.cluster.name,
                      style: Theme.of(context).textTheme.titleLarge,
                    ),
                    if (detail.cluster.summary.isNotEmpty) ...[
                      const SizedBox(height: 8),
                      Text(detail.cluster.summary),
                    ],
                    const SizedBox(height: 8),
                    Text(
                      '${detail.notes.length} note(s)',
                      style: Theme.of(context).textTheme.bodySmall,
                    ),
                  ],
                ),
              ),
              const Divider(height: 1),
              for (final note in detail.notes) _GroupedNote(note: note),
            ],
          );
        },
      ),
    );
  }
}

/// One note inside a group, with the marks a person should see.
class _GroupedNote extends StatelessWidget {
  const new({required this.note});

  final Note note;

  @override
  Widget build(BuildContext context) {
    final marks = <String>[
      if (note.metadata['tainted'] == true) 'from outside',
      if (note.metadata['transcribed'] == true) 'machine transcription',
      if (note.metadata['endorsed_at'] != null) 'endorsed',
    ];

    return ListTile(
      title: Text(note.content),
      subtitle: Text(
        [note.sourceType, if (marks.isNotEmpty) marks.join(' · ')].join(' • '),
      ),
      isThreeLine: marks.isNotEmpty,
    );
  }
}
