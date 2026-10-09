import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:http/http.dart' as http;

/// The store a browser uses.
RecordingStore newRecordingStore() => BlobRecordingStore();

/// [RecordingStore] over the `blob:` URL the recorder hands back.
///
/// Read from record_web 2.1.3's own source on 2026-10-09: on the web
/// `start(config, path:)` ignores the path and `stop()` returns
/// `URL.createObjectURL(blob)`. So there is no file here and never was; the
/// handle is that URL (ADR-0029).
class BlobRecordingStore implements RecordingStore {
  /// Creates a store. [client] is for a test; normally the default.
  new({http.Client? client}) : _client = client ?? http.Client();

  final http.Client _client;

  /// The bytes, read out of the blob as soon as the recording stops.
  ///
  /// A `blob:` URL lives as long as the document that made it, which is
  /// shorter than the life of an unsynced note; holding the bytes here means
  /// the URL's lifetime stops mattering for the rest of the session. It does
  /// not survive a reload, and ADR-0029 says so rather than pretending
  /// otherwise.
  final Map<String, RecordingBytes> _held = {};

  @override
  Future<String> newTarget() async {
    // Ignored by the recorder. Returned anyway so the caller has one shape
    // on both platforms, and so a log line says what it was for.
    return 'recording-${DateTime.now().millisecondsSinceEpoch}';
  }

  @override
  Future<int?> claim(String handle) async {
    if (!handle.startsWith('blob:')) return null;

    final http.Response response;
    try {
      // A blob: URL is fetchable from the document that created it, so the
      // ordinary client reads it without any browser interop.
      response = await _client.get(Uri.parse(handle));
    } on Exception {
      return null;
    }
    if (response.statusCode != 200 || response.bodyBytes.isEmpty) return null;

    // The blob carries its own type and it is not the one this app asked
    // for: a browser that cannot encode AAC records webm/opus instead.
    final mediaType =
        response.headers['content-type'] ?? 'application/octet-stream';
    _held[handle] = RecordingBytes(response.bodyBytes, mediaType);
    return response.bodyBytes.length;
  }

  @override
  Future<RecordingBytes?> read(String handle) async => _held[handle];

  @override
  Future<void> remove(String handle) async {
    _held.remove(handle);
    // The object URL is not revoked: that needs package:web for one call,
    // and the browser frees it when the page goes, which is also when the
    // bytes would have been lost anyway (ADR-0029).
  }
}
