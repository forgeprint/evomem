/// A device's way in, as the panel sees it (ADR-0030).
///
/// There is no secret here and there is no field for one: the server hands
/// it over once, when it is made, and keeps only a hash.
class DeviceToken {
  /// Creates a token.
  const new({
    required this.id,
    required this.name,
    required this.prefix,
    this.lastUsedAt,
    this.revokedAt,
  });

  /// Reads one out of what `GET /tokens` answered, or null when the shape
  /// is not one this version knows.
  static DeviceToken? fromJson(Object? raw) {
    if (raw is! Map<String, dynamic>) return null;
    final id = raw['id'];
    final name = raw['name'];
    if (id is! String || id.isEmpty || name is! String) return null;
    return DeviceToken(
      id: id,
      name: name,
      prefix: raw['prefix'] as String? ?? '',
      lastUsedAt: DateTime.tryParse(raw['last_used_at'] as String? ?? ''),
      revokedAt: DateTime.tryParse(raw['revoked_at'] as String? ?? ''),
    );
  }

  /// Identifies the token on the server.
  final String id;

  /// What somebody called the device.
  final String name;

  /// The part of the token kept in the clear, so one row can be told from
  /// another without the secret.
  final String prefix;

  /// When it was last used, or null when it never has been.
  ///
  /// This is what makes revoking safe rather than a guess: "not seen since
  /// March" is a reason, "I think that was the old phone" is not.
  final DateTime? lastUsedAt;

  /// When it was turned off, or null while it still works.
  final DateTime? revokedAt;

  /// Whether this token has been turned off.
  bool get isRevoked => revokedAt != null;
}
