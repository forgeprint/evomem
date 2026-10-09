import 'dart:developer' as developer;

import 'package:flutter/foundation.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// How bad an entry is.
enum LogLevel {
  /// Something happened that is worth knowing about.
  info,

  /// Something did not work, and the app carried on.
  warning,

  /// Something did not work and a person will notice.
  error,
}

/// One thing that happened.
class LogEntry {
  /// Creates an entry.
  const new({
    required this.at,
    required this.level,
    required this.source,
    required this.message,
    this.detail = '',
  });

  /// When it happened.
  final DateTime at;

  /// How bad it is.
  final LogLevel level;

  /// Which part of the app said it: `recording`, `sync`, `store`.
  final String source;

  /// What happened, in a sentence somebody can read.
  final String message;

  /// The machine's own words — an exception, a status line, a path.
  ///
  /// Kept apart from [message] because the first is for a person deciding
  /// what to do and the second is for whoever has to fix it.
  final String detail;

  /// One line, for a screen or the clipboard.
  @override
  String toString() {
    final time = at.toIso8601String();
    final tail = detail.isEmpty ? '' : ' — $detail';
    return '$time  ${level.name.toUpperCase()}  $source: $message$tail';
  }
}

/// How many entries are kept.
///
/// Enough to cover a session's worth of trying something and failing, few
/// enough that it is never the reason the app runs out of memory. The
/// oldest go first: what went wrong a minute ago is what somebody is
/// looking for.
const maxLogEntries = 300;

/// What the app has been doing, newest first.
///
/// Kept in memory and shown on a screen, because the places this has to be
/// read have no console: a browser hides it behind devtools and a phone has
/// none at all. Everything also goes to `dart:developer`, so `flutter run`
/// and the browser console still show it.
///
/// Nothing is written to disk. A log that outlives the session is a second
/// store to manage, and these entries can carry a server address or a note's
/// content in their detail.
class Logbook extends Notifier<List<LogEntry>> {
  @override
  List<LogEntry> build() => const [];

  /// Records something that happened.
  void add({
    required LogLevel level,
    required String source,
    required String message,
    Object? error,
    StackTrace? stackTrace,
  }) {
    final entry = LogEntry(
      at: DateTime.now(),
      level: level,
      source: source,
      message: message,
      detail: error == null ? '' : error.toString(),
    );

    developer.log(
      entry.message,
      time: entry.at,
      name: 'evomem.$source',
      level: switch (level) {
        LogLevel.info => 800,
        LogLevel.warning => 900,
        LogLevel.error => 1000,
      },
      error: error,
      stackTrace: stackTrace,
    );

    final kept = [entry, ...state];
    state = kept.length > maxLogEntries ? kept.sublist(0, maxLogEntries) : kept;
  }

  /// Notes something that worked, for following what the app did.
  void info(String source, String message) =>
      add(level: LogLevel.info, source: source, message: message);

  /// Notes something that did not work but was handled.
  void warn(String source, String message, [Object? error]) => add(
    level: LogLevel.warning,
    source: source,
    message: message,
    error: error,
  );

  /// Notes something that did not work and that a person will notice.
  void error(
    String source,
    String message, [
    Object? error,
    StackTrace? stackTrace,
  ]) => add(
    level: LogLevel.error,
    source: source,
    message: message,
    error: error,
    stackTrace: stackTrace,
  );

  /// Throws the entries away.
  void clear() => state = const [];
}

/// What the app has been doing.
final logbookProvider = NotifierProvider<Logbook, List<LogEntry>>(Logbook.new);

/// Sends what the Flutter framework and its packages print into [logbook].
///
/// `debugPrint` is where a package says what went wrong when it has decided
/// not to throw. record_web does exactly that: its MediaRecorder delegate
/// catches every failure in `start`, calls `debugPrint` and returns normally
/// — so the recording is already dead while the app is told it began, and
/// the only account of why is on a console nobody opens.
///
/// The previous printer is kept and still called, so `flutter run` and the
/// browser console show what they always did.
void routeDebugPrintTo(Logbook logbook) {
  if (_rerouted) return;
  _rerouted = true;

  final previous = debugPrint;
  debugPrint = (message, {wrapWidth}) {
    previous(message, wrapWidth: wrapWidth);
    if (message == null || message.trim().isEmpty) return;
    logbook.add(
      level: LogLevel.warning,
      source: 'framework',
      message: message.trim(),
    );
  };
}

bool _rerouted = false;
