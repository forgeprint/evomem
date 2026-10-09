import 'package:evomem_mobile/src/log/log.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';
import 'package:flutter_test/flutter_test.dart';

void main() {
  late ProviderContainer container;
  late Logbook log;

  setUp(() {
    container = ProviderContainer();
    addTearDown(container.dispose);
    log = container.read(logbookProvider.notifier);
  });

  test('newest first, because that is what somebody is looking for', () {
    log
      ..info('a', 'first')
      ..info('a', 'second');

    final entries = container.read(logbookProvider);
    expect(entries.map((e) => e.message).toList(), ['second', 'first']);
  });

  test('the error travels as its own text, apart from the message', () {
    log.error('recording', 'the recorder would not start', StateError('busy'));

    final entry = container.read(logbookProvider).single;
    expect(entry.level, LogLevel.error);
    expect(entry.source, 'recording');
    expect(entry.message, 'the recorder would not start');
    // The machine's words, which is the part worth copying into a report.
    expect(entry.detail, contains('busy'));
  });

  // Unbounded would make a long session the reason the app runs out of
  // memory, and the oldest entry is the one nobody is looking for.
  test('it keeps the newest and drops the oldest', () {
    for (var i = 0; i < maxLogEntries + 20; i++) {
      log.info('a', 'entry $i');
    }

    final entries = container.read(logbookProvider);
    expect(entries.length, maxLogEntries);
    expect(entries.first.message, 'entry ${maxLogEntries + 19}');
    expect(entries.last.message, 'entry 20');
  });

  test('clear empties it', () {
    log
      ..info('a', 'something')
      ..clear();
    expect(container.read(logbookProvider), isEmpty);
  });

  test('an entry reads as one line', () {
    log.warn('sync', 'could not reach the server', 'connection refused');
    final line = container.read(logbookProvider).single.toString();
    expect(line, contains('WARNING'));
    expect(line, contains('sync: could not reach the server'));
    expect(line, contains('connection refused'));
  });
}
