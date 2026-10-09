/// A grouping the server made over notes it holds.
///
/// Plain Dart, like `lib/src/rules`: a cluster has no behaviour of its own
/// and nothing here should need a widget to be tested.
class Cluster {
  /// Creates a cluster.
  const new({
    required this.id,
    required this.projectId,
    required this.name,
    required this.size,
    this.summary = '',
  });

  /// Reads one out of what `GET /clusters` answered.
  ///
  /// Returns null rather than throwing on a shape it does not recognise: this
  /// is another program's output, and one malformed entry should not lose the
  /// rest of the list.
  static Cluster? fromJson(Object? raw) {
    if (raw is! Map<String, dynamic>) return null;
    final id = raw['id'];
    final projectId = raw['project_id'];
    final name = raw['name'];
    if (id is! String || projectId is! String || name is! String) return null;
    if (id.isEmpty || name.isEmpty) return null;
    return Cluster(
      id: id,
      projectId: projectId,
      name: name,
      summary: raw['summary'] is String ? raw['summary'] as String : '',
      size: raw['size'] is int ? raw['size'] as int : 0,
    );
  }

  /// Identifies the cluster on the server.
  final String id;

  /// The project it groups notes in.
  final String projectId;

  /// What it is called.
  final String name;

  /// What the notes in it have in common, or empty.
  final String summary;

  /// How many notes are in it.
  ///
  /// Worth showing: a cluster of one and a cluster of everything are both
  /// usually mistakes, and a person scanning the list is the only check on
  /// whether the grouping was any good.
  final int size;
}
