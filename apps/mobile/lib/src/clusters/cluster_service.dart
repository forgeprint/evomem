import 'dart:convert';

import 'package:evomem_mobile/src/clusters/cluster.dart';
import 'package:evomem_mobile/src/http/utf8_body.dart';
import 'package:evomem_mobile/src/rules/note.dart';
import 'package:http/http.dart' as http;

/// Why there are no clusters to show.
enum ClusterProblem {
  /// No server address or token has been entered in settings.
  notConfigured,

  /// The server could not be reached, or refused.
  unreachable,

  /// The server answered something this cannot read.
  unreadable,
}

/// A failure with a reason a screen can explain.
class ClusterFailure implements Exception {
  /// Creates a failure.
  const new(this.problem, [this.detail = '']);

  /// Why it failed.
  final ClusterProblem problem;

  /// What the server or the client said, when there was anything.
  final String detail;

  /// Says what went wrong, in words.
  ///
  /// Without this a release build logs `Instance of 'minified:fn'`, which
  /// is what the log said the first time somebody needed it. The default
  /// Object.toString is the class name, and release builds have no class
  /// names.
  @override
  String toString() {
    final reason = switch (problem) {
      ClusterProblem.notConfigured => 'no server address or token in settings',
      ClusterProblem.unreachable => 'the server could not be reached',
      ClusterProblem.unreadable => 'the server answered something unreadable',
    };
    return detail.isEmpty ? reason : '$reason: $detail';
  }
}

/// One cluster and the notes in it.
class ClusterDetail {
  /// Creates a detail.
  const new({required this.cluster, required this.notes});

  /// The grouping.
  final Cluster cluster;

  /// The notes it names, newest first.
  final List<Note> notes;
}

/// Reads the groupings the server made.
///
/// There is no local copy and there is not meant to be one. A note is
/// authored on a device and pushed; a cluster is derived on the server from a
/// view of everything and lives only there (ADR-0024). That is what lets the
/// phone show clusters without two-way sync — and why a phone with no signal
/// shows none.
class ClusterService {
  /// Reads from [serverUrl] with [apiToken].
  const new({
    required this.serverUrl,
    required this.apiToken,
    required this.client,
    this.timeout = const Duration(seconds: 15),
  });

  /// Where the server is, as entered in settings.
  final String serverUrl;

  /// The bearer token the read side is behind.
  final String apiToken;

  /// The client to send with. Injected so a test needs no network.
  final http.Client client;

  /// How long one request may take.
  final Duration timeout;

  /// Whether there is anything to ask.
  bool get isConfigured => serverUrl.isNotEmpty && apiToken.isNotEmpty;

  /// The groupings in a project, most recently changed first.
  Future<List<Cluster>> list({required String projectId}) async {
    final body = await _get(
      '/clusters?project=${Uri.encodeQueryComponent(projectId)}',
    );
    final raw = body['clusters'];
    if (raw is! List) {
      throw const ClusterFailure(
        ClusterProblem.unreadable,
        'no clusters array',
      );
    }
    // One unreadable entry drops out rather than losing the whole list.
    return raw.map(Cluster.fromJson).whereType<Cluster>().toList();
  }

  /// One grouping and the notes in it.
  Future<ClusterDetail> get(String id) async =>
      _detailFrom(await _get('/clusters/${Uri.encodeComponent(id)}'));

  /// Reads a grouping and its notes out of a reply.
  ///
  /// Shared by reading one and editing one, because the server answers an
  /// edit with what the grouping now is — so the caller never has to guess,
  /// and there is one place that knows the shape.
  static ClusterDetail _detailFrom(Map<String, dynamic> body) {
    final cluster = Cluster.fromJson(body['cluster']);
    if (cluster == null) {
      throw const ClusterFailure(
        ClusterProblem.unreadable,
        'no cluster object',
      );
    }
    final rawNotes = body['notes'];
    final notes = <Note>[];
    if (rawNotes is List) {
      for (final one in rawNotes) {
        // One unreadable entry drops out rather than losing the rest.
        notes.addAll([?_noteFromJson(one)]);
      }
    }
    return ClusterDetail(cluster: cluster, notes: notes);
  }

  /// Renames a grouping, re-summarises it, or changes who is in it.
  ///
  /// Null means "leave it": renaming a group must not wipe its summary, and
  /// an empty string must still be able to clear one. That is why these are
  /// nullable rather than defaulted.
  ///
  /// A cluster still lives on the server and nowhere else (ADR-0024); this
  /// asks the server to change one, which is what the MCP agent already
  /// does. What a note *says* is not touched here and cannot be.
  Future<ClusterDetail> edit(
    String id, {
    String? name,
    String? summary,
    List<String> remove = const [],
  }) async {
    // Null means "leave it alone"; an empty string is a deliberate clear.
    // The server tells them apart because the field is absent rather than
    // empty, which is why these are built conditionally.
    final body = <String, Object?>{
      'name': ?name,
      'summary': ?summary,
      if (remove.isNotEmpty) 'remove': remove,
    };
    if (body.isEmpty) {
      throw const ClusterFailure(
        ClusterProblem.unreadable,
        'nothing to change',
      );
    }
    final answer = await _send(
      'PUT',
      '/clusters/${Uri.encodeComponent(id)}',
      body: body,
    );
    return _detailFrom(answer);
  }

  /// Takes a grouping off. The notes stay — that is the whole reason this
  /// is safe to offer (ADR-0023).
  Future<void> delete(String id) async {
    await _send('DELETE', '/clusters/${Uri.encodeComponent(id)}');
  }

  Future<Map<String, dynamic>> _send(
    String method,
    String path, {
    Map<String, Object?>? body,
  }) async {
    if (!isConfigured) {
      throw const ClusterFailure(ClusterProblem.notConfigured);
    }

    final request = http.Request(method, Uri.parse('$serverUrl$path'))
      ..headers['Authorization'] = 'Bearer $apiToken';
    if (body != null) {
      request.headers['Content-Type'] = 'application/json';
      request.body = json.encode(body);
    }

    final http.Response response;
    try {
      final streamed = await client.send(request).timeout(timeout);
      response = await http.Response.fromStream(streamed);
    } on Exception catch (e) {
      throw ClusterFailure(ClusterProblem.unreachable, e.toString());
    }

    if (response.statusCode < 200 || response.statusCode >= 300) {
      // The server's own sentence: it says things the client cannot know,
      // like which field was empty.
      throw ClusterFailure(
        ClusterProblem.unreachable,
        utf8Body(response).trim().isEmpty
            ? 'the server answered ${response.statusCode}'
            : utf8Body(response).trim(),
      );
    }

    if (utf8Body(response).trim().isEmpty) return const {};
    try {
      final decoded = json.decode(utf8Body(response));
      if (decoded is! Map<String, dynamic>) {
        throw const ClusterFailure(ClusterProblem.unreadable, 'not an object');
      }
      return decoded;
    } on FormatException catch (e) {
      throw ClusterFailure(ClusterProblem.unreadable, e.message);
    }
  }

  Future<Map<String, dynamic>> _get(String path) async {
    if (!isConfigured) {
      throw const ClusterFailure(ClusterProblem.notConfigured);
    }

    final http.Response response;
    try {
      response = await client
          .get(
            Uri.parse('$serverUrl$path'),
            headers: {'Authorization': 'Bearer $apiToken'},
          )
          .timeout(timeout);
    } on Exception catch (e) {
      throw ClusterFailure(ClusterProblem.unreachable, e.toString());
    }

    if (response.statusCode != 200) {
      throw ClusterFailure(
        ClusterProblem.unreachable,
        'the server answered ${response.statusCode}',
      );
    }

    try {
      final decoded = json.decode(utf8Body(response));
      if (decoded is! Map<String, dynamic>) {
        throw const ClusterFailure(ClusterProblem.unreadable, 'not an object');
      }
      return decoded;
    } on FormatException catch (e) {
      throw ClusterFailure(ClusterProblem.unreadable, e.message);
    }
  }

  /// Reads a note out of what the read side answered.
  ///
  /// The server's shape, not the phone's: `source_type`, `created_at` and a
  /// `metadata` object that carries the marks a person is shown.
  static Note? _noteFromJson(Object? raw) {
    if (raw is! Map<String, dynamic>) return null;
    final id = raw['id'];
    final content = raw['content'];
    if (id is! String || content is! String) return null;
    final created = DateTime.tryParse(raw['created_at'] as String? ?? '');
    final updated = DateTime.tryParse(raw['updated_at'] as String? ?? '');
    if (created == null || updated == null) return null;
    return Note(
      id: id,
      projectId: raw['project_id'] as String? ?? '',
      content: content,
      sourceType: raw['source_type'] as String? ?? 'manual',
      createdAt: created,
      updatedAt: updated,
      metadata: raw['metadata'] is Map<String, dynamic>
          ? raw['metadata'] as Map<String, dynamic>
          : const {},
    );
  }
}
