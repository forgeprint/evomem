import 'package:flutter/material.dart';
import 'package:go_router/go_router.dart';

/// The way out of a screen that was reached from another one.
///
/// Every screen here is navigated to with `context.go`, which replaces what
/// is on screen rather than stacking on it — so Flutter puts no back button
/// in the app bar. In a browser that was invisible: the browser's own back
/// button went back. On a phone there is no such button, and the first time
/// the app ran on one, the Log screen was a room with no door.
///
/// It takes the destination rather than popping, because there is nothing to
/// pop: `go` left no stack behind.
class BackTo extends StatelessWidget {
  /// Creates a back button that goes to [location].
  const new(this.location, {super.key});

  /// Where back is, as a route's `location`.
  final String location;

  @override
  Widget build(BuildContext context) => IconButton(
    icon: const Icon(Icons.arrow_back),
    tooltip: MaterialLocalizations.of(context).backButtonTooltip,
    onPressed: () => context.go(location),
  );
}
