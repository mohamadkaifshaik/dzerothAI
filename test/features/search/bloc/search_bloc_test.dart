import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/post/domain/entities/post.dart';
import 'package:dzeroth/features/search/domain/entities/user_search_result.dart';
import 'package:dzeroth/features/search/domain/repositories/search_repository.dart';
import 'package:dzeroth/features/search/presentation/bloc/search_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockSearchRepository extends Mock implements SearchRepository {}

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

const _author = PostAuthor(
  id: '01900000-0000-7000-8000-000000000001',
  handle: 'alice',
  displayName: 'Alice',
);

Post _makePost(String id) => Post(
  id: id,
  author: _author,
  postType: 'original',
  content: 'Post $id',
  isDeleted: false,
  createdAt: DateTime.utc(2024),
  updatedAt: DateTime.utc(2024),
);

UserSearchResult _makeUser(String id) =>
    UserSearchResult(id: id, handle: 'user_$id', displayName: 'User $id');

final _post1 = _makePost('post-1');
final _post2 = _makePost('post-2');
final _post3 = _makePost('post-3');

final _user1 = _makeUser('user-1');
final _user2 = _makeUser('user-2');
final _user3 = _makeUser('user-3');

SearchPostPage _postPageWithMore(
  List<Post> items, {
  String cursor = 'cursor-2',
}) => SearchPostPage(items: items, nextCursor: cursor, terminated: false);

SearchPostPage _terminatedPostPage(List<Post> items) =>
    SearchPostPage(items: items, nextCursor: null, terminated: true);

SearchUserPage _userPageWithMore(
  List<UserSearchResult> items, {
  String cursor = 'cursor-u2',
}) => SearchUserPage(items: items, nextCursor: cursor, terminated: false);

SearchUserPage _terminatedUserPage(List<UserSearchResult> items) =>
    SearchUserPage(items: items, nextCursor: null, terminated: true);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockSearchRepository mockRepo;

  setUp(() {
    mockRepo = MockSearchRepository();
  });

  // Shorten to avoid waiting for the 300 ms debounce in every test.
  // _SearchQueryDebounced is the internal event used after debounce. We bypass
  // the debounce in tests by dispatching _SearchQueryDebounced directly or by
  // seeding state and dispatching next-page events.
  //
  // For tests that exercise SearchQueryChanged we use a fake-async approach or
  // we simply call SearchUsersRequested / next-page events which are not
  // debounced.

  group('SearchBloc — SearchUsersRequested', () {
    // -----------------------------------------------------------------------
    // SearchUsersRequested — non-terminated
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'SearchBloc_EmitsLoading_ThenUsersLoaded_OnUsersRequested: '
      'emits [SearchLoading, SearchUsersLoaded] for non-terminated user page',
      build: () {
        when(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        ).thenAnswer((_) async => Success(_userPageWithMore([_user1, _user2])));
        return SearchBloc(searchRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const SearchUsersRequested(query: 'alice')),
      expect: () => [
        const SearchLoading(query: 'alice'),
        SearchUsersLoaded(
          users: [_user1, _user2],
          nextCursor: 'cursor-u2',
          hasMore: true,
          query: 'alice',
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.searchUsers(query: 'alice', cursor: null))
            .called(1);
      },
    );

    // -----------------------------------------------------------------------
    // SearchUsersRequested — terminated immediately
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'SearchBloc_EmitsUsersTerminated_WhenTerminatedFlagTrue: '
      'emits [SearchLoading, SearchUsersTerminated]',
      build: () {
        when(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        ).thenAnswer((_) async => Success(_terminatedUserPage([_user1])));
        return SearchBloc(searchRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const SearchUsersRequested(query: 'alice')),
      expect: () => [
        const SearchLoading(query: 'alice'),
        SearchUsersTerminated(users: [_user1], query: 'alice'),
      ],
    );

    // -----------------------------------------------------------------------
    // SearchUsersRequested — empty
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'emits [SearchLoading, SearchEmpty] when user search returns empty list',
      build: () {
        when(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        ).thenAnswer(
          (_) async => const Success(
            SearchUserPage(items: [], nextCursor: null, terminated: false),
          ),
        );
        return SearchBloc(searchRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const SearchUsersRequested(query: 'nobody')),
      expect: () => [
        const SearchLoading(query: 'nobody'),
        const SearchEmpty(query: 'nobody', searchType: 'users'),
      ],
    );

    // -----------------------------------------------------------------------
    // SearchUsersRequested — error
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'emits [SearchLoading, SearchError] when user search fails',
      build: () {
        when(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        ).thenAnswer((_) async => const Err(NetworkFailure('unreachable')));
        return SearchBloc(searchRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const SearchUsersRequested(query: 'alice')),
      expect: () => [
        const SearchLoading(query: 'alice'),
        isA<SearchError>().having((s) => s.message, 'message', isNotEmpty),
      ],
    );

    // -----------------------------------------------------------------------
    // SearchUsersRequested — empty query is ignored
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'emits nothing when SearchUsersRequested has empty query',
      build: () => SearchBloc(searchRepository: mockRepo),
      act: (bloc) => bloc.add(const SearchUsersRequested(query: '')),
      expect: () => <SearchState>[],
      verify: (_) {
        verifyNever(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        );
      },
    );
  });

  group('SearchBloc — SearchUsersNextPageRequested', () {
    // -----------------------------------------------------------------------
    // Next user page — appends users
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'SearchBloc_LoadsNextUserPage: '
      'fetches next page and appends users when hasMore is true',
      build: () {
        when(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        ).thenAnswer(
          (_) async =>
              Success(_userPageWithMore([_user3], cursor: 'cursor-u3')),
        );
        return SearchBloc(searchRepository: mockRepo);
      },
      seed: () => SearchUsersLoaded(
        users: [_user1, _user2],
        nextCursor: 'cursor-u2',
        hasMore: true,
        query: 'alice',
      ),
      act: (bloc) => bloc.add(const SearchUsersNextPageRequested()),
      expect: () => [
        // In-flight: hasMore locked to false.
        SearchUsersLoaded(
          users: [_user1, _user2],
          nextCursor: 'cursor-u2',
          hasMore: false,
          query: 'alice',
        ),
        // Final: next page merged.
        SearchUsersLoaded(
          users: [_user1, _user2, _user3],
          nextCursor: 'cursor-u3',
          hasMore: true,
          query: 'alice',
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.searchUsers(query: 'alice', cursor: 'cursor-u2'))
            .called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Next user page terminated — merged + terminated state
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'emits SearchUsersTerminated with merged users when next page returns '
      'terminated:true',
      build: () {
        when(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        ).thenAnswer((_) async => Success(_terminatedUserPage([_user3])));
        return SearchBloc(searchRepository: mockRepo);
      },
      seed: () => SearchUsersLoaded(
        users: [_user1, _user2],
        nextCursor: 'cursor-u2',
        hasMore: true,
        query: 'alice',
      ),
      act: (bloc) => bloc.add(const SearchUsersNextPageRequested()),
      expect: () => [
        SearchUsersLoaded(
          users: [_user1, _user2],
          nextCursor: 'cursor-u2',
          hasMore: false,
          query: 'alice',
        ),
        SearchUsersTerminated(users: [_user1, _user2, _user3], query: 'alice'),
      ],
    );

    // -----------------------------------------------------------------------
    // SearchUsersTerminated is terminal — next page ignored
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'SearchBloc_IgnoresNextUserPage_AfterTerminated: '
      'SearchUsersNextPageRequested after SearchUsersTerminated emits nothing',
      build: () => SearchBloc(searchRepository: mockRepo),
      seed: () => SearchUsersTerminated(users: [_user1], query: 'alice'),
      act: (bloc) => bloc.add(const SearchUsersNextPageRequested()),
      expect: () => <SearchState>[],
      verify: (_) {
        verifyNever(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        );
      },
    );

    // -----------------------------------------------------------------------
    // hasMore = false guard
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'does nothing when hasMore is false (duplicate user next-page guard)',
      build: () => SearchBloc(searchRepository: mockRepo),
      seed: () => SearchUsersLoaded(
        users: [_user1],
        nextCursor: null,
        hasMore: false,
        query: 'alice',
      ),
      act: (bloc) => bloc.add(const SearchUsersNextPageRequested()),
      expect: () => <SearchState>[],
      verify: (_) {
        verifyNever(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        );
      },
    );
  });

  group('SearchBloc — SearchPostsNextPageRequested', () {
    // -----------------------------------------------------------------------
    // Next post page — appends posts
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'SearchBloc_LoadsNextPostPage: '
      'fetches next page and appends posts when hasMore is true',
      build: () {
        when(
          () => mockRepo.searchPosts(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        ).thenAnswer(
          (_) async => Success(_postPageWithMore([_post3], cursor: 'cursor-3')),
        );
        return SearchBloc(searchRepository: mockRepo);
      },
      seed: () => SearchPostsLoaded(
        posts: [_post1, _post2],
        nextCursor: 'cursor-2',
        hasMore: true,
        query: 'flutter',
      ),
      act: (bloc) => bloc.add(const SearchPostsNextPageRequested()),
      expect: () => [
        // In-flight: hasMore locked to false.
        SearchPostsLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: false,
          query: 'flutter',
        ),
        // Final: merged.
        SearchPostsLoaded(
          posts: [_post1, _post2, _post3],
          nextCursor: 'cursor-3',
          hasMore: true,
          query: 'flutter',
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.searchPosts(query: 'flutter', cursor: 'cursor-2'))
            .called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Next post page terminated — merged + terminated state
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'emits SearchPostsTerminated with merged posts when next page returns '
      'terminated:true',
      build: () {
        when(
          () => mockRepo.searchPosts(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        ).thenAnswer((_) async => Success(_terminatedPostPage([_post3])));
        return SearchBloc(searchRepository: mockRepo);
      },
      seed: () => SearchPostsLoaded(
        posts: [_post1, _post2],
        nextCursor: 'cursor-2',
        hasMore: true,
        query: 'flutter',
      ),
      act: (bloc) => bloc.add(const SearchPostsNextPageRequested()),
      expect: () => [
        SearchPostsLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: false,
          query: 'flutter',
        ),
        SearchPostsTerminated(
          posts: [_post1, _post2, _post3],
          query: 'flutter',
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // SearchPostsTerminated is terminal — next page ignored
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'SearchBloc_IgnoresNextPostPage_AfterTerminated: '
      'SearchPostsNextPageRequested after SearchPostsTerminated emits nothing',
      build: () => SearchBloc(searchRepository: mockRepo),
      seed: () => SearchPostsTerminated(posts: [_post1], query: 'flutter'),
      act: (bloc) => bloc.add(const SearchPostsNextPageRequested()),
      expect: () => <SearchState>[],
      verify: (_) {
        verifyNever(
          () => mockRepo.searchPosts(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        );
      },
    );

    // -----------------------------------------------------------------------
    // hasMore = false guard
    // -----------------------------------------------------------------------

    blocTest<SearchBloc, SearchState>(
      'does nothing when hasMore is false (duplicate post next-page guard)',
      build: () => SearchBloc(searchRepository: mockRepo),
      seed: () => SearchPostsLoaded(
        posts: [_post1],
        nextCursor: null,
        hasMore: false,
        query: 'flutter',
      ),
      act: (bloc) => bloc.add(const SearchPostsNextPageRequested()),
      expect: () => <SearchState>[],
      verify: (_) {
        verifyNever(
          () => mockRepo.searchPosts(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        );
      },
    );
  });

  group('SearchBloc — SearchQueryChanged (empty query → SearchInitial)', () {
    // -----------------------------------------------------------------------
    // Empty query immediately resets to SearchInitial
    // -----------------------------------------------------------------------

    // Note: SearchQueryChanged is debounced by a Timer. Empty query is handled
    // synchronously (timer is cancelled and SearchInitial is emitted directly).
    // bloc_test does not wait for timers — we verify the empty-query short-
    // circuit by dispatching an empty query.
    blocTest<SearchBloc, SearchState>(
      'SearchBloc_EmitsInitial_OnEmptyQuery: '
      'emits SearchInitial immediately when query is empty',
      build: () => SearchBloc(searchRepository: mockRepo),
      act: (bloc) => bloc.add(const SearchQueryChanged(query: '')),
      expect: () => [const SearchInitial()],
      verify: (_) {
        verifyNever(
          () => mockRepo.searchPosts(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        );
        verifyNever(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        );
      },
    );

    blocTest<SearchBloc, SearchState>(
      'emits SearchInitial when whitespace-only query is provided',
      build: () => SearchBloc(searchRepository: mockRepo),
      act: (bloc) => bloc.add(const SearchQueryChanged(query: '   ')),
      expect: () => [const SearchInitial()],
      verify: (_) {
        verifyNever(
          () => mockRepo.searchPosts(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        );
      },
    );
  });

  group('SearchBloc — next-page error recovery', () {
    blocTest<SearchBloc, SearchState>(
      'restores SearchPostsLoaded and emits SearchError when next-page post '
      'fetch fails',
      build: () {
        when(
          () => mockRepo.searchPosts(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        ).thenAnswer((_) async => const Err(NetworkFailure('offline')));
        return SearchBloc(searchRepository: mockRepo);
      },
      seed: () => SearchPostsLoaded(
        posts: [_post1, _post2],
        nextCursor: 'cursor-2',
        hasMore: true,
        query: 'flutter',
      ),
      act: (bloc) => bloc.add(const SearchPostsNextPageRequested()),
      expect: () => [
        // In-flight.
        SearchPostsLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: false,
          query: 'flutter',
        ),
        // Restored so user can retry.
        SearchPostsLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: true,
          query: 'flutter',
        ),
        isA<SearchError>(),
      ],
    );

    blocTest<SearchBloc, SearchState>(
      'restores SearchUsersLoaded and emits SearchError when next-page user '
      'fetch fails',
      build: () {
        when(
          () => mockRepo.searchUsers(
            query: any(named: 'query'),
            cursor: any(named: 'cursor'),
          ),
        ).thenAnswer((_) async => const Err(ServerFailure('internal error')));
        return SearchBloc(searchRepository: mockRepo);
      },
      seed: () => SearchUsersLoaded(
        users: [_user1, _user2],
        nextCursor: 'cursor-u2',
        hasMore: true,
        query: 'alice',
      ),
      act: (bloc) => bloc.add(const SearchUsersNextPageRequested()),
      expect: () => [
        SearchUsersLoaded(
          users: [_user1, _user2],
          nextCursor: 'cursor-u2',
          hasMore: false,
          query: 'alice',
        ),
        SearchUsersLoaded(
          users: [_user1, _user2],
          nextCursor: 'cursor-u2',
          hasMore: true,
          query: 'alice',
        ),
        isA<SearchError>(),
      ],
    );
  });
}
