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
