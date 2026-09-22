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

/// The title library request failed.
final class TitleLibraryError extends TitleLibraryState {
  const TitleLibraryError(this.failure);

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}
