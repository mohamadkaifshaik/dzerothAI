part of 'title_library_bloc.dart';

/// All events that can be dispatched to [TitleLibraryBloc].
sealed class TitleLibraryEvent {
  const TitleLibraryEvent();
}

/// Load (or reload) the authenticated user's title library.
final class LoadTitleLibrary extends TitleLibraryEvent {
  const LoadTitleLibrary();
}

/// Set the authenticated user's primary title to [userTitleId].
final class SetPrimaryTitle extends TitleLibraryEvent {
  const SetPrimaryTitle(this.userTitleId);

  final String userTitleId;
}

/// Clear the authenticated user's primary title.
final class ClearPrimaryTitle extends TitleLibraryEvent {
  const ClearPrimaryTitle();
}
