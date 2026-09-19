import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/post_analytics.dart';
import '../../domain/repositories/studio_repository.dart';

part 'studio_event.dart';
part 'studio_state.dart';

/// Manages the paginated Creator Studio analytics list for the authenticated user.
///
/// Per CLAUDE.md §2.1 there is NO infinite scrolling.
/// When the repository returns [AnalyticsPage.terminated] == true, this BLoC
/// emits [StudioTerminated] and silently ignores all subsequent
/// [StudioNextPageRequested] events.
///
/// Per CLAUDE.md §2.3, the analytics counts surfaced here are PRIVATE.
/// They must only be rendered inside [StudioScreen].
class StudioBloc extends Bloc<StudioEvent, StudioState> {
  StudioBloc({required StudioRepository studioRepository})
    : _repository = studioRepository,
      super(const StudioInitial()) {
    on<StudioFetchRequested>(_onFetchRequested);
    on<StudioNextPageRequested>(_onNextPageRequested);
  }

  final StudioRepository _repository;

  Future<void> _onFetchRequested(
    StudioFetchRequested event,
    Emitter<StudioState> emit,
  ) async {
    emit(const StudioLoading());

    final result = await _repository.getAnalytics(cursor: null);

    switch (result) {
      case Success(:final value):
        if (value.terminated) {
          if (value.items.isEmpty) {
            emit(const StudioEmpty());
          } else {
            emit(StudioTerminated(items: value.items));
          }
        } else {
          emit(
            StudioLoaded(
              items: value.items,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
            ),
          );
        }
      case Err(:final failure):
        emit(StudioError(failure: failure));
    }
  }

  Future<void> _onNextPageRequested(
    StudioNextPageRequested event,
    Emitter<StudioState> emit,
  ) async {
    // Critical invariant: once terminated, reject all further page requests.
    final current = state;
    if (current is StudioTerminated) return;
    if (current is! StudioLoaded) return;
    if (!current.hasMore) return;

    // Lock hasMore to false while in-flight to prevent duplicate requests.
    emit(
      StudioLoaded(
        items: current.items,
        nextCursor: current.nextCursor,
        hasMore: false,
      ),
    );

    final result = await _repository.getAnalytics(cursor: current.nextCursor);

    switch (result) {
      case Success(:final value):
        final merged = [...current.items, ...value.items];
        if (value.terminated) {
          emit(StudioTerminated(items: merged));
        } else {
          emit(
            StudioLoaded(
              items: merged,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
            ),
          );
        }
      case Err(:final failure):
        // Restore previous loaded state so the user can retry.
        emit(
          StudioLoaded(
            items: current.items,
            nextCursor: current.nextCursor,
            hasMore: current.nextCursor != null,
          ),
        );
        emit(StudioError(failure: failure));
    }
  }
}
