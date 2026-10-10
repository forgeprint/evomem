import 'dart:convert';

import 'package:http/http.dart' as http;

/// What a reply says, read as UTF-8 whatever its headers claim.
///
/// `response.body` is not this. It decodes [http.Response.bodyBytes] with the
/// `charset` parameter of `Content-Type`, and **falls back to latin-1** when
/// there is none — documented in http 1.3.0 as RFC 2616 behaviour (package
/// source, read 2026-10-10). The server used to send `application/json` with
/// no charset, so every Turkish character arrived mangled: `küme` read back
/// as `kÃ¼me`, on every screen that showed a note.
///
/// The server declares the charset now. This does not depend on it. JSON is
/// UTF-8 by definition, the two fixes are independent, and only one of them
/// is in this repository's gift once somebody runs an older server.
///
/// Malformed bytes become U+FFFD rather than throwing: a reply that is almost
/// readable is worth showing, and a decoder that threw would turn one bad
/// byte into a blank screen.
String utf8Body(http.Response response) =>
    const Utf8Decoder(allowMalformed: true).convert(response.bodyBytes);
