import 'dart:async';

import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/result.dart';
import '../../../post/domain/entities/post.dart';
import '../../domain/entities/user_search_result.dart';
import '../../domain/repositories/search_repository.dart';

part 'search_event.dart';
part 'search_state.dart';

/// Manages search state for the Dzeroth search screen.
///
/// The BLoC handles two independent result tabs (Posts and Users). Each tab
/// supports cursor-paginated loading that terminates at the server-enforced
/// boundary of 50 items per CLAUDE.md §2.1.
///
/// Debounce:
/// [SearchQueryChanged] is debounced by 300 ms via an internal [Timer].
/// A new query clears all previous results and starts fresh. An empty query
/// emits [SearchInitial] without making any network request.
///
/// Terminal invariant:
/// [SearchPostsTerminated] and [SearchUsersTerminated] are hard terminal
/// states. Further [SearchPostsNextPageRequested] or
/// [SearchUsersNextPageRequested] events for the same query are silently
/// ignored once the boundary is reached.
///
/// Public metrics lockdown:
/// No social-validation metric (likes, impressions, bookmark counts, follower
/// counts, etc.) is ever carried through this BLoC per CLAUDE.md §2.3.
class SearchBloc extends Bloc<SearchEvent, SearchState> {
  SearchBloc({required SearchRepository searchRepository})
    : _repository = searchRepository,
      super(const SearchInitial()) {
    on<SearchQueryChanged>(_onQueryChanged);
    on<_SearchQueryDebounced>(_onQueryDebounced);
    on<SearchUsersRequested>(_onUsersRequested);
    on<SearchPostsNextPageRequested>(_onPostsNextPageRequested);
    on<SearchUsersNextPageRequested>(_onUsersNextPageRequested);
  }

  final SearchRepository _repository;
  Timer? _debounceTimer;

  /// The query that the most recent search request was made for.
  /// Used to prevent stale responses from overwriting newer results.
  String _activeQuery = '';

  @override
  Future<void> close() {
    _debounceTimer?.cancel();
    return super.close();
  }

  // ---------------------------------------------------------------------------
  // Event handlers
  // ---------------------------------------------------------------------------

  void _onQueryChanged(SearchQueryChanged event, Emitter<SearchState> emit) {
    _debounceTimer?.cancel();

    final query = event.query.trim();

    if (query.isEmpty) {
      _activeQuery = '';
      emit(const SearchInitial());
      return;
    }

    // Debounce: fire the actual search after 300 ms of inactivity.
    _debounceTimer = Timer(
      const Duration(milliseconds: 300),
      () => add(_SearchQueryDebounced(query: query)),
    );
  }

  Future<void> _onQueryDebounced(
    _SearchQueryDebounced event,
    Emitter<SearchState> emit,
  ) async {
    final query = event.query;
    _activeQuery = query;
    emit(SearchLoading(query: query));

    final result = await _repository.searchPosts(query: query, cursor: null);

    // Guard: discard if the user has typed a newer query while in-flight.
    if (_activeQuery != query) return;

    switch (result) {
      case Success(:final value):
        if (value.terminated) {
          if (value.items.isEmpty) {
            emit(SearchEmpty(query: query, searchType: 'posts'));
          } else {
            emit(SearchPostsTerminated(posts: value.items, query: query));
          }
        } else if (value.items.isEmpty) {
          emit(SearchEmpty(query: query, searchType: 'posts'));
        } else {
          emit(
            SearchPostsLoaded(
              posts: value.items,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
              query: query,
            ),
          );
        }
      case Err(:final failure):
        emit(SearchError(message: failure.message, query: query));
    }
  }

  Future<void> _onUsersRequested(
    SearchUsersRequested event,
    Emitter<SearchState> emit,
  ) async {
    final query = event.query.trim();
    if (query.isEmpty) return;

    _activeQuery = query;
    emit(SearchLoading(query: query));

    final result = await _repository.searchUsers(query: query, cursor: null);

    if (_activeQuery != query) return;

    switch (result) {
      case Success(:final value):
        if (value.terminated) {
          if (value.items.isEmpty) {
            emit(SearchEmpty(query: query, searchType: 'users'));
          } else {
            emit(SearchUsersTerminated(users: value.items, query: query));
          }
        } else if (value.items.isEmpty) {
          emit(SearchEmpty(query: query, searchType: 'users'));
        } else {
          emit(
            SearchUsersLoaded(
              users: value.items,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
              query: query,
            ),
          );
        }
      case Err(:final failure):
        emit(SearchError(message: failure.message, query: query));
    }
  }

  Future<void> _onPostsNextPageRequested(
    SearchPostsNextPageRequested event,
    Emitter<SearchState> emit,
  ) async {
    final current = state;

    // Terminal invariant: once terminated, reject all further page requests.
    if (current is SearchPostsTerminated) return;
    if (current is! SearchPostsLoaded) return;
    if (!current.hasMore) return;

    final query = current.query;

    // Sync _activeQuery so the stale-result guard does not discard valid
    // responses when the handler is entered from a seeded state (e.g. tests).
    _activeQuery = query;

    // Lock hasMore to false while in-flight to prevent duplicate requests.
    emit(
      SearchPostsLoaded(
        posts: current.posts,
        nextCursor: current.nextCursor,
        hasMore: false,
        query: query,
      ),
    );

    final result = await _repository.searchPosts(
      query: query,
      cursor: current.nextCursor,
    );

    // Guard: discard if the active query has changed.
    if (_activeQuery != query) return;

    switch (result) {
      case Success(:final value):
        final merged = [...current.posts, ...value.items];
        if (value.terminated) {
          emit(SearchPostsTerminated(posts: merged, query: query));
        } else {
          emit(
            SearchPostsLoaded(
              posts: merged,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
              query: query,
            ),
          );
        }
      case Err(:final failure):
        // Restore previous loaded state so the user can retry.
        emit(
          SearchPostsLoaded(
            posts: current.posts,
            nextCursor: current.nextCursor,
            hasMore: current.nextCursor != null,
            query: query,
          ),
        );
        emit(SearchError(message: failure.message, query: query));
    }
  }

  Future<void> _onUsersNextPageRequested(
    SearchUsersNextPageRequested event,
    Emitter<SearchState> emit,
  ) async {
    final current = state;

    // Terminal invariant: once terminated, reject all further page requests.
    if (current is SearchUsersTerminated) return;
    if (current is! SearchUsersLoaded) return;
    if (!current.hasMore) return;

    final query = current.query;

    // Sync _activeQuery so the stale-result guard does not discard valid
    // responses when the handler is entered from a seeded state (e.g. tests).
    _activeQuery = query;

    // Lock hasMore to false while in-flight to prevent duplicate requests.
    emit(
      SearchUsersLoaded(
        users: current.users,
        nextCursor: current.nextCursor,
        hasMore: false,
        query: query,
      ),
    );

    final result = await _repository.searchUsers(
      query: query,
      cursor: current.nextCursor,
    );

    // Guard: discard if the active query has changed.
    if (_activeQuery != query) return;

    switch (result) {
      case Success(:final value):
        final merged = [...current.users, ...value.items];
        if (value.terminated) {
          emit(SearchUsersTerminated(users: merged, query: query));
        } else {
          emit(
            SearchUsersLoaded(
              users: merged,
              nextCursor: value.nextCursor,
              hasMore: value.nextCursor != null,
              query: query,
            ),
          );
        }
      case Err(:final failure):
        // Restore previous loaded state so the user can retry.
        emit(
          SearchUsersLoaded(
            users: current.users,
            nextCursor: current.nextCursor,
            hasMore: current.nextCursor != null,
            query: query,
          ),
        );
        emit(SearchError(message: failure.message, query: query));
    }
  }
}
