import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/user_title.dart';
import '../../domain/repositories/title_repository.dart';

part 'title_library_event.dart';
part 'title_library_state.dart';

/// Manages the authenticated user's title library.
///
/// Loads all owned titles and the current primary title selection.
/// Allows the user to set or clear their primary title.
///
/// Per CLAUDE.md §2.3, private management data (category, status,
/// is_revocable) is only displayed within this authenticated screen and must
/// never appear in any public feed, profile, or search result.
class TitleLibraryBloc extends Bloc<TitleLibraryEvent, TitleLibraryState> {
  TitleLibraryBloc({required TitleRepository titleRepository})
    : _repository = titleRepository,
      super(const TitleLibraryInitial()) {
    on<LoadTitleLibrary>(_onLoad);
    on<SetPrimaryTitle>(_onSetPrimary);
    on<ClearPrimaryTitle>(_onClearPrimary);
  }

  final TitleRepository _repository;

  Future<void> _onLoad(
    LoadTitleLibrary event,
    Emitter<TitleLibraryState> emit,
  ) async {
    emit(const TitleLibraryLoading());

    final result = await _repository.getMyTitles();

    switch (result) {
      case Success(:final value):
        emit(
          TitleLibraryLoaded(titles: value.titles, primaryId: value.primaryId),
        );
      case Err(:final failure):
        emit(TitleLibraryError(failure));
    }
  }

  Future<void> _onSetPrimary(
    SetPrimaryTitle event,
    Emitter<TitleLibraryState> emit,
  ) async {
    final current = state;
    if (current is! TitleLibraryLoaded) return;

    // Optimistically update primaryId so the UI reflects the change immediately.
    emit(TitleLibraryLoaded(titles: current.titles, primaryId: event.userTitleId));

    final result = await _repository.setPrimaryTitle(event.userTitleId);

    switch (result) {
      case Success():
        // Primary is already reflected in the optimistic update.
        // Re-emit the loaded state with confirmed primaryId.
        emit(
          TitleLibraryLoaded(
            titles: current.titles,
            primaryId: event.userTitleId,
          ),
        );
      case Err(:final failure):
        // Revert to previous primary on failure.
        emit(TitleLibraryLoaded(titles: current.titles, primaryId: current.primaryId));
        emit(TitleLibraryError(failure));
    }
  }

  Future<void> _onClearPrimary(
    ClearPrimaryTitle event,
    Emitter<TitleLibraryState> emit,
  ) async {
    final current = state;
    if (current is! TitleLibraryLoaded) return;

    // Optimistically clear primary.
    emit(TitleLibraryLoaded(titles: current.titles, primaryId: null));

    final result = await _repository.clearPrimaryTitle();

    switch (result) {
      case Success():
        // Already reflected — nothing more to do.
        break;
      case Err(:final failure):
        // Revert to previous primary on failure.
        emit(TitleLibraryLoaded(titles: current.titles, primaryId: current.primaryId));
        emit(TitleLibraryError(failure));
    }
  }
}
