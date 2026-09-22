part of 'title_library_bloc.dart';

/// All states that [TitleLibraryBloc] can emit.
sealed class TitleLibraryState extends Equatable {
  const TitleLibraryState();

  @override
  List<Object?> get props => [];
}

/// No titles have been requested yet.
final class TitleLibraryInitial extends TitleLibraryState {
  const TitleLibraryInitial();
}

/// The title library is loading.
final class TitleLibraryLoading extends TitleLibraryState {
  const TitleLibraryLoading();
}

/// Titles have been loaded successfully.
///
/// [primaryId] is the [UserTitle.id] of the active primary title, or null
/// when no primary is set.
final class TitleLibraryLoaded extends TitleLibraryState {
  const TitleLibraryLoaded({required this.titles, required this.primaryId});

  final List<UserTitle> titles;
  final String? primaryId;

  @override
  List<Object?> get props => [titles, primaryId];
}

/// A set/clear mutation is in-flight.
///
/// The current [titles] and [primaryId] are preserved from the last confirmed
/// server state so the UI can show the existing selection alongside a loading
/// indicator without prematurely reflecting the requested change.
///
/// This state is emitted BEFORE any API call is made and replaced with either
/// [TitleLibraryLoaded] (on success, after server confirmation via getMyTitles)
/// or [TitleLibraryError] (on failure).
final class TitleLibraryMutating extends TitleLibraryState {
  const TitleLibraryMutating({required this.titles, required this.primaryId});

  final List<UserTitle> titles;
  final String? primaryId;

  @override
  List<Object?> get props => [titles, primaryId];
}

/// The title library request failed.
final class TitleLibraryError extends TitleLibraryState {
  const TitleLibraryError(this.failure);

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
