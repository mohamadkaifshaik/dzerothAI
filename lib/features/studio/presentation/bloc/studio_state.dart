part of 'studio_bloc.dart';

/// All states that [StudioBloc] can emit.
sealed class StudioState extends Equatable {
  const StudioState();

  @override
  List<Object?> get props => [];
}

/// No analytics have been requested yet.
final class StudioInitial extends StudioState {
  const StudioInitial();
}

/// The first page of analytics is loading.
final class StudioLoading extends StudioState {
  const StudioLoading();
}

/// At least one page has been loaded and the list is not yet terminated.
///
/// [hasMore] is false while a next-page request is in-flight, preventing
/// duplicate requests.
final class StudioLoaded extends StudioState {
  const StudioLoaded({
    required this.items,
    required this.nextCursor,
    required this.hasMore,
  });

  final List<PostAnalytics> items;
  final String? nextCursor;

  /// True when [nextCursor] is non-null and no next-page request is in-flight.
  final bool hasMore;

  @override
  List<Object?> get props => [items, nextCursor, hasMore];
}

/// The analytics list has reached the server-enforced boundary.
///
/// Per CLAUDE.md §2.1 this is a terminal state. No further pages must be
/// fetched. The UI must show [GoTouchGrassWidget].
final class StudioTerminated extends StudioState {
  const StudioTerminated({required this.items});

  final List<PostAnalytics> items;

  @override
  List<Object?> get props => [items];
}

/// The authenticated user has no posts — the analytics list is empty.
///
/// Distinct from [StudioTerminated]: this state represents "no content" rather
/// than "all available content has been shown".
final class StudioEmpty extends StudioState {
  const StudioEmpty();
}

/// The analytics request failed.
final class StudioError extends StudioState {
  const StudioError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
