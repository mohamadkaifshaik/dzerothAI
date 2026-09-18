import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/bookmark/domain/entities/bookmark_item.dart';
import 'package:dzeroth/features/bookmark/domain/entities/bookmark_page.dart';
import 'package:dzeroth/features/bookmark/domain/repositories/bookmark_repository.dart';
import 'package:dzeroth/features/bookmark/presentation/bloc/bookmark_list_bloc.dart';
import 'package:dzeroth/features/post/domain/entities/post.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockBookmarkRepository extends Mock implements BookmarkRepository {}

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

BookmarkItem _makeItem(String postId) => BookmarkItem(
  postId: postId,
  createdAt: DateTime.utc(2024),
  post: _makePost(postId),
);

final _post1 = _makePost('post-1');
final _post2 = _makePost('post-2');
final _post3 = _makePost('post-3');

final _item1 = _makeItem('post-1');
final _item2 = _makeItem('post-2');
final _item3 = _makeItem('post-3');

BookmarkPage _pageWithMore(
  List<BookmarkItem> items, {
  String cursor = 'cursor-2',
}) => BookmarkPage(items: items, nextCursor: cursor, terminated: false);

BookmarkPage _terminatedPage(List<BookmarkItem> items) =>
    BookmarkPage(items: items, nextCursor: null, terminated: true);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockBookmarkRepository mockRepo;

  setUp(() {
    mockRepo = MockBookmarkRepository();
  });

  group('BookmarkListBloc', () {
    // -----------------------------------------------------------------------
    // Load → terminated immediately
    // -----------------------------------------------------------------------

    blocTest<BookmarkListBloc, BookmarkListState>(
      'BookmarkListBloc_EmitsTerminated_WhenTerminatedFlagTrue: '
      'emits [BookmarkListLoading, BookmarkListTerminated] '
      'when repository returns terminated:true',
      build: () {
        when(
          () => mockRepo.listBookmarks(cursor: any(named: 'cursor')),
        ).thenAnswer((_) async => Success(_terminatedPage([_item1, _item2])));
        return BookmarkListBloc(bookmarkRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const BookmarkListLoadRequested()),
      expect: () => [
        const BookmarkListLoading(),
        BookmarkListTerminated(posts: [_post1, _post2]),
      ],
      verify: (_) {
        verify(() => mockRepo.listBookmarks(cursor: null)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // BookmarkListTerminated is terminal — next page ignored
    // -----------------------------------------------------------------------

    blocTest<BookmarkListBloc, BookmarkListState>(
      'BookmarkListBloc_IgnoresNextPage_AfterTerminated: '
      'BookmarkListNextPageRequested after BookmarkListTerminated emits nothing '
      'and does not call the repository',
      build: () => BookmarkListBloc(bookmarkRepository: mockRepo),
      seed: () => BookmarkListTerminated(posts: [_post1]),
      act: (bloc) => bloc.add(const BookmarkListNextPageRequested()),
      // Terminal state: no emissions.
      expect: () => <BookmarkListState>[],
      verify: (_) {
        verifyNever(() => mockRepo.listBookmarks(cursor: any(named: 'cursor')));
      },
    );

    // -----------------------------------------------------------------------
    // Load next page → appends posts
    // -----------------------------------------------------------------------

    blocTest<BookmarkListBloc, BookmarkListState>(
      'BookmarkListBloc_LoadsNextPage: '
      'fetches next page and appends posts when hasMore is true',
      build: () {
        when(() => mockRepo.listBookmarks(cursor: any(named: 'cursor')))
            .thenAnswer(
              (_) async => Success(_pageWithMore([_item3], cursor: 'cursor-3')),
            );
        return BookmarkListBloc(bookmarkRepository: mockRepo);
      },
      seed: () => BookmarkListLoaded(
        posts: [_post1, _post2],
        nextCursor: 'cursor-2',
        hasMore: true,
      ),
      act: (bloc) => bloc.add(const BookmarkListNextPageRequested()),
      expect: () => [
        // In-flight: hasMore locked to false.
        BookmarkListLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: false,
        ),
        // Final: next page merged.
        BookmarkListLoaded(
          posts: [_post1, _post2, _post3],
          nextCursor: 'cursor-3',
          hasMore: true,
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.listBookmarks(cursor: 'cursor-2')).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Load first page → non-terminated
    // -----------------------------------------------------------------------

    blocTest<BookmarkListBloc, BookmarkListState>(
      'emits [BookmarkListLoading, BookmarkListLoaded] on successful '
      'non-terminated load',
      build: () {
        when(() => mockRepo.listBookmarks(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => Success(_pageWithMore([_item1, _item2])));
        return BookmarkListBloc(bookmarkRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const BookmarkListLoadRequested()),
      expect: () => [
        const BookmarkListLoading(),
        BookmarkListLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: true,
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // Repository failure → BookmarkListError
    // -----------------------------------------------------------------------

    blocTest<BookmarkListBloc, BookmarkListState>(
      'emits [BookmarkListLoading, BookmarkListError] when repository returns '
      'a failure',
      build: () {
        when(() => mockRepo.listBookmarks(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => const Err(NetworkFailure('unreachable')));
        return BookmarkListBloc(bookmarkRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const BookmarkListLoadRequested()),
      expect: () => [
        const BookmarkListLoading(),
        isA<BookmarkListError>().having(
          (s) => s.failure,
          'failure',
          isA<NetworkFailure>(),
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // hasMore = false guard (duplicate next-page blocked)
    // -----------------------------------------------------------------------

    blocTest<BookmarkListBloc, BookmarkListState>(
      'does nothing when hasMore is false (duplicate request guard)',
      build: () => BookmarkListBloc(bookmarkRepository: mockRepo),
      seed: () =>
          BookmarkListLoaded(posts: [_post1], nextCursor: null, hasMore: false),
      act: (bloc) => bloc.add(const BookmarkListNextPageRequested()),
      expect: () => <BookmarkListState>[],
      verify: (_) {
        verifyNever(() => mockRepo.listBookmarks(cursor: any(named: 'cursor')));
      },
    );

    // -----------------------------------------------------------------------
    // Next page terminated → merged + terminated state
    // -----------------------------------------------------------------------

    blocTest<BookmarkListBloc, BookmarkListState>(
      'emits BookmarkListTerminated with merged posts when next page returns '
      'terminated:true',
      build: () {
        when(() => mockRepo.listBookmarks(cursor: any(named: 'cursor')))
            .thenAnswer((_) async => Success(_terminatedPage([_item3])));
        return BookmarkListBloc(bookmarkRepository: mockRepo);
      },
      seed: () => BookmarkListLoaded(
        posts: [_post1, _post2],
        nextCursor: 'cursor-2',
        hasMore: true,
      ),
      act: (bloc) => bloc.add(const BookmarkListNextPageRequested()),
      expect: () => [
        BookmarkListLoaded(
          posts: [_post1, _post2],
          nextCursor: 'cursor-2',
          hasMore: false,
        ),
        BookmarkListTerminated(posts: [_post1, _post2, _post3]),
      ],
    );
  });
}
