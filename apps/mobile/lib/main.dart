import 'package:evomem_mobile/src/app.dart';
import 'package:evomem_mobile/src/log/log.dart';
import 'package:flutter/widgets.dart';
import 'package:flutter_riverpod/flutter_riverpod.dart';

/// Starts the app inside the one [ProviderScope] every provider is read from.
void main() {
  final container = ProviderContainer();

  // Before the first frame, so a package that complains during start-up is
  // caught. The logbook is read here rather than lazily because what it
  // exists to catch happens before anything else reads it.
  routeDebugPrintTo(container.read(logbookProvider.notifier));

  runApp(
    UncontrolledProviderScope(container: container, child: const EvomemApp()),
  );
}
