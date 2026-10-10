import 'package:evomem_mobile/src/clusters/cluster_providers.dart';
import 'package:evomem_mobile/src/clusters/cluster_service.dart';
import 'package:evomem_mobile/src/routing/routes.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:evomem_mobile/src/ui/back_to.dart';
import 'package:evomem_mobile/src/ui/clusters_screen.dart'
    show explainClusterProblem;
import 'package:flutter/material.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:go_router/go_router.dart';

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
  bool _busy = false;

  @override
  void initState() {
    super.initState();
    _detail = _load();
  }

  Future<ClusterDetail> _load() async {
    final service = await ref.read(clusterServiceProvider.future);
    return await service.get(widget.clusterId);
  }

  void _say(String message) {
    if (!mounted) return;
    ScaffoldMessenger.of(context)
        .showSnackBar(SnackBar(content: Text(message)));
  }

  /// Runs something that changes the grouping on the server.
  ///
  /// The reply carries what the grouping now is, so the screen is told
  /// rather than asked again — and a failure says the server's own words,
  /// which know things this does not.
  Future<void> _change(
    Future<ClusterDetail> Function(ClusterService) act,
  ) async {
    setState(() => _busy = true);
    try {
      final service = await ref.read(clusterServiceProvider.future);
      final changed = await act(service);
      if (!mounted) return;
      setState(() => _detail = Future.value(changed));
      // The list behind this screen holds names and sizes, both of which
      // may have just moved.
      ref.invalidate(clustersProvider);
    } on ClusterFailure catch (failure) {
      _say(
        failure.detail.isEmpty
            ? explainClusterProblem(failure.problem)
            : failure.detail,
      );
    } finally {
      if (mounted) setState(() => _busy = false);
    }
  }

  Future<void> _rename() async {
    final current = await _detail;
    if (!mounted) return;

    final name = TextEditingController(text: current.cluster.name);
    final summary = TextEditingController(text: current.cluster.summary);
    final saved = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Rename this group'),
        content: Column(
          mainAxisSize: MainAxisSize.min,
          children: [
            TextField(
              controller: name,
              autofocus: true,
              decoration: const InputDecoration(
                labelText: 'Name',
                border: OutlineInputBorder(),
              ),
            ),
            const SizedBox(height: 12),
            TextField(
              controller: summary,
              maxLines: 3,
              decoration: const InputDecoration(
                labelText: 'Summary',
                helperText: 'What this group is about. May be empty.',
                border: OutlineInputBorder(),
              ),
            ),
          ],
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Save'),
          ),
        ],
      ),
    );
    if (saved != true) return;

    await _change(
      (service) => service.edit(
        widget.clusterId,
        name: name.text.trim(),
        summary: summary.text.trim(),
      ),
    );
  }

  /// Takes a note out of the group. The note itself is untouched — that is
  /// the whole reason this is safe to offer (ADR-0023).
  Future<void> _remove(Note note) async {
    await _change(
      (service) => service.edit(widget.clusterId, remove: [note.id]),
    );
    _say('taken out of the group; the note is still there');
  }

  Future<void> _delete() async {
    final confirmed = await showDialog<bool>(
      context: context,
      builder: (context) => AlertDialog(
        title: const Text('Delete this group?'),
        content: const Text(
          'The grouping goes and the notes stay. An agent can make it '
          'again, or you can.',
        ),
        actions: [
          TextButton(
            onPressed: () => Navigator.of(context).pop(false),
            child: const Text('Cancel'),
          ),
          FilledButton(
            onPressed: () => Navigator.of(context).pop(true),
            child: const Text('Delete the group'),
          ),
        ],
      ),
    );
    if (confirmed != true) return;

    setState(() => _busy = true);
    try {
      final service = await ref.read(clusterServiceProvider.future);
      await service.delete(widget.clusterId);
      if (!mounted) return;
      ref.invalidate(clustersProvider);
      context.go(const ClustersRoute().location);
    } on ClusterFailure catch (failure) {
      _say(
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
    return Scaffold(
      appBar: AppBar(
        title: const Text('Group'),
        leading: BackTo(const ClustersRoute().location),
        actions: [
          IconButton(
            tooltip: 'Rename',
            icon: const Icon(Icons.edit_outlined),
            onPressed: _busy ? null : _rename,
          ),
          IconButton(
            tooltip: 'Delete this group',
            icon: const Icon(Icons.delete_outline),
            onPressed: _busy ? null : _delete,
          ),
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
              for (final note in detail.notes)
                _GroupedNote(
                  note: note,
                  onRemove: _busy ? null : () => _remove(note),
                ),
            ],
          );
        },
      ),
    );
  }
}

/// One note inside a group, with the marks a person should see.
class _GroupedNote extends StatelessWidget {
  const new({required this.note, this.onRemove});

  final Note note;

  /// Takes it out of the group, or null while something else is running.
  final VoidCallback? onRemove;

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
      trailing: IconButton(
        tooltip: 'Take out of this group',
        icon: const Icon(Icons.playlist_remove),
        onPressed: onRemove,
      ),
    );
  }
}
