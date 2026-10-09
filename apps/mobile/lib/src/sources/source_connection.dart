/// A source the server pulls from, as the panel sees it.
///
/// There is no token here and there is no field for one: the server never
/// sends it back, and a shape that could hold it is a shape that could leak
/// it (ADR-0026).
class SourceConnection {
  /// Creates a connection.
  const new({
    required this.id,
    required this.sourceType,
    required this.projectId,
    required this.baseUrl,
    this.account = '',
    this.query = '',
    this.lastPulledAt,
    this.lastError = '',
  });

  /// Reads one out of what `GET /connections` answered, or null when the
  /// shape is not one this version knows.
  static SourceConnection? fromJson(Object? raw) {
    if (raw is! Map<String, dynamic>) return null;
    final id = raw['id'];
    final sourceType = raw['source_type'];
    final projectId = raw['project_id'];
    if (id is! String || sourceType is! String || projectId is! String) {
      return null;
    }
    if (id.isEmpty) return null;
    return SourceConnection(
      id: id,
      sourceType: sourceType,
      projectId: projectId,
      baseUrl: raw['base_url'] as String? ?? '',
      account: raw['account'] as String? ?? '',
      query: raw['query'] as String? ?? '',
      lastPulledAt: DateTime.tryParse(raw['last_pulled_at'] as String? ?? ''),
      lastError: raw['last_error'] as String? ?? '',
    );
  }

  /// Identifies the connection on the server.
  final String id;

  /// Which source it is: jira, and whatever comes next.
  final String sourceType;

  /// The project pulled notes are filed under.
  final String projectId;

  /// The source's base address.
  final String baseUrl;

  /// The account the token belongs to, when the source wants one.
  final String account;

  /// What the source is asked for — JQL, for Jira.
  final String query;

  /// When it was last pulled, or null when it never has been.
  final DateTime? lastPulledAt;

  /// Why the last pull stopped, or empty when it did not.
  ///
  /// Kept on the connection rather than only in a log, because a log has
  /// rotated by the time somebody asks why a source is stale.
  final String lastError;

  /// Whether this row is a model's API key rather than a source.
  ///
  /// The server keeps both in one keyring (ADR-0027). The panel has to tell
  /// them apart because a model is never pulled from and a source never
  /// groups anything.
  bool get isModel => sourceType == modelSourceType;

  /// Which model a model row names. The server puts it in the query field,
  /// which for a model means which model.
  String get modelName => query;
}

/// What the server calls a row in the keyring that holds a model key.
const modelSourceType = 'model';

/// What one grouping run did.
class OrganizeOutcome {
  /// Creates an outcome.
  const new({
    required this.offered,
    required this.created,
    required this.grouped,
    required this.invented,
    required this.dropped,
  });

  /// Reads it out of what `POST /organize` answered.
  factory fromJson(Object? raw) {
    final map = raw is Map<String, dynamic> ? raw : const <String, dynamic>{};
    int at(String key) => map[key] is int ? map[key] as int : 0;
    return OrganizeOutcome(
      offered: at('offered'),
      created: at('created'),
      grouped: at('grouped'),
      invented: at('invented'),
      dropped: at('dropped'),
    );
  }

  /// How many ungrouped notes the model was given.
  final int offered;

  /// How many groups were written.
  final int created;

  /// How many notes ended up in one.
  final int grouped;

  /// How many ids the model named that were never sent to it.
  ///
  /// Shown rather than swallowed: a model that invents half its ids is one
  /// pointed at the wrong task, and the number is the only sign of it.
  final int invented;

  /// How many groups came back with nothing usable in them.
  final int dropped;
}

/// What one pull did.
class PullOutcome {
  /// Creates an outcome.
  const new({
    required this.considered,
    required this.pulled,
    required this.failed,
  });

  /// Reads it out of what `POST /connections/pull` answered.
  factory fromJson(Object? raw) {
    final map = raw is Map<String, dynamic> ? raw : const <String, dynamic>{};
    int at(String key) => map[key] is int ? map[key] as int : 0;
    return PullOutcome(
      considered: at('considered'),
      pulled: at('pulled'),
      failed: at('failed'),
    );
  }

  /// How many sources were tried.
  final int considered;

  /// How many notes arrived.
  final int pulled;

  /// How many sources reported a problem.
  final int failed;
}
