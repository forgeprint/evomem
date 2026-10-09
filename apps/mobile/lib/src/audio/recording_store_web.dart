import 'dart:js_interop';

import 'package:evomem_mobile/src/audio/recording_store.dart';
import 'package:http/http.dart' as http;
import 'package:web/web.dart' as web;

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

/// Plays back through an audio element.
RecordingPlayer newRecordingPlayer() => BlobRecordingPlayer();

/// [RecordingPlayer] over an `<audio>` element pointed at the blob URL.
///
/// The browser already has everything needed to play its own blob, so this
/// is the whole implementation: no package, no bytes copied, and the same
/// handle the recorder handed back.
class BlobRecordingPlayer implements RecordingPlayer {
  web.HTMLAudioElement? _element;
  String? _url;

  @override
  bool get canPlay => true;

  @override
  Future<void> play(RecordingBytes recording) async {
    await stop();

    // Its own blob, built from the bytes it was handed. The recorder's
    // blob is not reused: these bytes may have come from the server, and
    // one path for both is one path to get wrong.
    final blob = web.Blob(
      [recording.bytes.toJS].toJS,
      web.BlobPropertyBag(type: recording.mediaType),
    );
    final url = web.URL.createObjectURL(blob);
    final element = web.HTMLAudioElement()
      ..src = url
      ..preload = 'auto';
    _url = url;
    _element = element;
    await element.play().toDart;
  }

  @override
  Future<void> stop() async {
    final element = _element;
    if (element != null) {
      element
        ..pause()
        ..removeAttribute('src')
        ..load();
      _element = null;
    }
    // Revoked, unlike the recorder's own URL: this one was made here, so
    // leaving it would hold a copy of the audio for the life of the page
    // every time anybody pressed play.
    final url = _url;
    if (url != null) {
      web.URL.revokeObjectURL(url);
      _url = null;
    }
  }
}

/// Asks the browser why the microphone is not available.
///
/// Three different things come back as one refusal and they need different
/// fixes: the page is not a secure context, the permission is denied, or
/// there is no input device. Each is asked separately so the answer names
/// which — that is the difference between a log somebody can act on and one
/// that only says something went wrong.
///
/// Every call is guarded on its own. A browser that will not answer one of
/// them should still report the other two.
Future<String> describeMicrophone() async {
  final parts = <String>[];

  if (!web.window.isSecureContext) {
    parts.add(
      'the page is not a secure context, so no browser will give it a '
      'microphone; serve it over https or from localhost',
    );
  }

  try {
    final status = await web.window.navigator.permissions
        .query(_PermissionDescriptor(name: 'microphone'))
        .toDart;
    parts.add('permission is ${status.state}');
  } on Object {
    parts.add('the browser would not say what the permission is');
  }

  try {
    final devices = await web.window.navigator.mediaDevices
        .enumerateDevices()
        .toDart;
    final inputs = devices.toDart.where((d) => d.kind == 'audioinput').length;
    parts.add('$inputs audio input(s)');
  } on Object {
    parts.add('the devices could not be listed');
  }

  return parts.join('; ');
}

/// The shape `permissions.query` takes. Written out because package:web
/// does not expose one; the same workaround record_web uses.
extension type _PermissionDescriptor._(JSObject _) implements JSObject {
  external factory({required String name});
}
