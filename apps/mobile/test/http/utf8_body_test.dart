import 'dart:convert';

import 'package:evomem_mobile/src/http/utf8_body.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:http/http.dart' as http;

/// A reply with the given bytes and headers, the way a server sends one.
http.Response reply(String text, {String? contentType}) => http.Response.bytes(
  utf8.encode(text),
  200,
  headers: {'content-type': ?contentType},
);

void main() {
  // The app read every note through `response.body`, which falls back to
  // latin-1 when the Content-Type carries no charset — documented in
  // http 1.3.0 as RFC 2616 behaviour. The server sent `application/json`
  // with no charset, so `küme testi` came back as `kÃ¼me testi` on screen.
  //
  // The JSON half of that was fixed upstream. http's own CHANGELOG, 1.4.0:
  // "Fixed default encoding for application/json without a charset to use
  // utf8 instead of latin1". The previous version of this test asserted
  // that `response.body` got JSON wrong and said "if this ever passes, the
  // http package changed its default and the whole guard can go". It passed
  // the day dependabot moved http from 1.3.0 to 1.6.0, which is how this
  // was found — a red build on main rather than a silent change of meaning.
  //
  // The guard does not go, because the prediction was only half right.
  // Measured against http 1.6.0 on 2026-10-12: `application/json` is read
  // as UTF-8 now, but `text/plain`, `application/octet-stream` and no
  // Content-Type at all are all still latin-1, and `utf8Body` also passes
  // `allowMalformed`, which the package never does.
  group('a reply is read as UTF-8', () {
    // The case the mojibake actually came from. This no longer needs
    // utf8Body, and is kept because it is the one the server sends.
    test('for JSON, which the package itself now gets right', () {
      final response = reply('küme testi', contentType: 'application/json');

      expect(response.body, 'küme testi', reason: 'http >= 1.4.0');
      expect(utf8Body(response), 'küme testi');
    });

    // Why the guard stays. If this ever fails, the package has started
    // defaulting to UTF-8 everywhere and utf8Body is down to allowMalformed.
    test('and for the types the package still reads as latin-1', () {
      for (final contentType in [
        'text/plain',
        'application/octet-stream',
        null,
      ]) {
        final response = reply('küme testi', contentType: contentType);

        expect(
          response.body,
          isNot('küme testi'),
          reason: 'content-type: ${contentType ?? '<none>'}',
        );
        expect(
          utf8Body(response),
          'küme testi',
          reason: 'content-type: ${contentType ?? '<none>'}',
        );
      }
    });

    test('and when it says so properly', () {
      expect(
        utf8Body(reply('küme', contentType: 'application/json; charset=utf-8')),
        'küme',
      );
    });

    test('and when it sends no Content-Type at all', () {
      expect(utf8Body(reply('ğüşıöçĞÜŞİÖÇ')), 'ğüşıöçĞÜŞİÖÇ');
    });

    // The characters this project has been careful about all along: the
    // dotless ı is the one that breaks search, and it is the one most
    // likely to be mangled unnoticed (ADR-0003).
    test('the Turkish letters survive a round trip', () {
      const text = 'veritabanı ışık çöğüş İIı';
      expect(utf8Body(reply(text, contentType: 'application/json')), text);
    });

    test('a malformed byte does not blank the whole reply', () {
      // One stray byte in the middle of otherwise fine text.
      final response = http.Response.bytes([
        ...utf8.encode('before '),
        0xC3,
        ...utf8.encode(' after'),
      ], 200);

      final body = utf8Body(response);
      expect(body, contains('before'));
      expect(body, contains('after'));
    });
  });
}
