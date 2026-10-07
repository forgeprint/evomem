// ignore: unused_import
import 'package:intl/intl.dart' as intl;

import 'app_localizations.dart';

// ignore_for_file: type=lint

/// The translations for English (`en`).
class AppLocalizationsEn extends AppLocalizations {
  AppLocalizationsEn([String locale = 'en']) : super(locale);

  @override
  String get appTitle => 'Evomem';

  @override
  String get homeScreenTitle => 'Memory';

  @override
  String get searchHint => 'Search notes...';

  @override
  String get addNote => 'Add Note';

  @override
  String get noteContentLabel => 'Note content';

  @override
  String get saveNote => 'Save';

  @override
  String get cancel => 'Cancel';

  @override
  String get noteSaved => 'Note saved';

  @override
  String get noteDeleted => 'Note deleted';

  @override
  String get deleteNote => 'Delete note';

  @override
  String get deleteConfirm => 'Are you sure you want to delete this note?';

  @override
  String get projectLabel => 'Project';

  @override
  String get sourceTypeLabel => 'Source';

  @override
  String get settings => 'Settings';

  @override
  String get syncStatus => 'Sync Status';

  @override
  String lastSynced(String time) {
    return 'Last synced: $time';
  }

  @override
  String get syncNow => 'Sync Now';

  @override
  String get syncing => 'Syncing...';

  @override
  String syncFailed(String error) {
    return 'Sync failed: $error';
  }

  @override
  String get noNotes => 'No notes yet. Tap + to add one.';

  @override
  String noteTooLong(int max) {
    return 'Note is too long (max $max characters).';
  }

  @override
  String get noteBlank => 'Note cannot be empty.';

  @override
  String get openNote => 'Open note';

  @override
  String get backToList => 'Back to list';

  @override
  String get noteNotFound => 'Note not found.';

  @override
  String get pageMissing => 'That page does not exist.';
}
