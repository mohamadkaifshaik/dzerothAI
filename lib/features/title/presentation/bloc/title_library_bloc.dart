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

    // Pessimistic: show the current (unchanged) titles while the request is
    // in-flight. The primaryId shown during mutation is the OLD confirmed value
    // — NOT the newly requested one.
    emit(TitleLibraryMutating(titles: current.titles, primaryId: current.primaryId));

    final setResult = await _repository.setPrimaryTitle(event.userTitleId);

    switch (setResult) {
      case Err(:final failure):
        // API call failed — restore the prior confirmed state and surface error.
        emit(TitleLibraryLoaded(titles: current.titles, primaryId: current.primaryId));
        emit(TitleLibraryError(failure));
        return;
      case Success():
        break;
    }

    // API call succeeded — fetch the server-confirmed library to determine the
    // actual new primaryId. The backend is the source of truth.
    final getResult = await _repository.getMyTitles();

    switch (getResult) {
      case Success(:final value):
        emit(TitleLibraryLoaded(titles: value.titles, primaryId: value.primaryId));
      case Err(:final failure):
        // Cannot confirm server state — do NOT fabricate a local primaryId.
        emit(TitleLibraryError(failure));
    }
  }

  Future<void> _onClearPrimary(
    ClearPrimaryTitle event,
    Emitter<TitleLibraryState> emit,
  ) async {
    final current = state;
    if (current is! TitleLibraryLoaded) return;

    // Pessimistic: preserve current confirmed state during the API call.
    emit(TitleLibraryMutating(titles: current.titles, primaryId: current.primaryId));

    final clearResult = await _repository.clearPrimaryTitle();

    switch (clearResult) {
      case Err(:final failure):
        // API call failed — restore the prior confirmed state and surface error.
        emit(TitleLibraryLoaded(titles: current.titles, primaryId: current.primaryId));
        emit(TitleLibraryError(failure));
        return;
      case Success():
        break;
    }

    // API call succeeded — fetch the server-confirmed library.
    final getResult = await _repository.getMyTitles();

    switch (getResult) {
      case Success(:final value):
        emit(TitleLibraryLoaded(titles: value.titles, primaryId: value.primaryId));
      case Err(:final failure):
        // Cannot confirm server state — do NOT fabricate a local primaryId.
        emit(TitleLibraryError(failure));
    }
  }
}
