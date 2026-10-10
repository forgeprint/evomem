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
  group('a reply is read as UTF-8', () {
    test('even when the server says nothing about the charset', () {
      final response = reply('küme testi', contentType: 'application/json');

      expect(
        response.body,
        isNot('küme testi'),
        reason: 'if this ever passes, the http package changed its default '
            'and the whole guard can go',
      );
      expect(utf8Body(response), 'küme testi');
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
      final response = http.Response.bytes(
        [...utf8.encode('before '), 0xC3, ...utf8.encode(' after')],
        200,
      );

      final body = utf8Body(response);
      expect(body, contains('before'));
      expect(body, contains('after'));
    });
  });
}
