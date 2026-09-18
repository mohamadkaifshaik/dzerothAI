import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/bookmark/domain/repositories/bookmark_repository.dart';
import 'package:dzeroth/features/bookmark/presentation/bloc/bookmark_toggle_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockBookmarkRepository extends Mock implements BookmarkRepository {}

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

const _postId = 'post-1';

BookmarkToggleBloc _makeBloc(MockBookmarkRepository repo) =>
    BookmarkToggleBloc(bookmarkRepository: repo, postId: _postId);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockBookmarkRepository mockRepo;

  setUp(() {
    mockRepo = MockBookmarkRepository();
  });

  group('BookmarkToggleBloc', () {
    // -----------------------------------------------------------------------
    // BookmarkRequested → success
    // -----------------------------------------------------------------------

    blocTest<BookmarkToggleBloc, BookmarkToggleState>(
      'BookmarkToggleBloc_EmitsSuccess_OnBookmarkRequested: '
      'emits [BookmarkLoading, BookmarkSuccess(isBookmarked: true)] on success',
      build: () {
        when(() => mockRepo.bookmark(_postId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const BookmarkRequested()),
      expect: () => [
        const BookmarkLoading(),
        const BookmarkSuccess(isBookmarked: true),
      ],
      verify: (_) {
        verify(() => mockRepo.bookmark(_postId)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // UnbookmarkRequested → success
    // -----------------------------------------------------------------------

    blocTest<BookmarkToggleBloc, BookmarkToggleState>(
      'BookmarkToggleBloc_EmitsSuccess_OnUnbookmarkRequested: '
      'emits [BookmarkLoading, BookmarkSuccess(isBookmarked: false)] on success',
      build: () {
        when(() => mockRepo.unbookmark(_postId))
            .thenAnswer((_) async => const Success(null));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const UnbookmarkRequested()),
      expect: () => [
        const BookmarkLoading(),
        const BookmarkSuccess(isBookmarked: false),
      ],
      verify: (_) {
        verify(() => mockRepo.unbookmark(_postId)).called(1);
      },
    );

    // -----------------------------------------------------------------------
    // Repository failure → BookmarkError
    // -----------------------------------------------------------------------

    blocTest<BookmarkToggleBloc, BookmarkToggleState>(
      'BookmarkToggleBloc_EmitsError_OnFailure: '
      'emits [BookmarkLoading, BookmarkError] when bookmark returns a failure',
      build: () {
        when(() => mockRepo.bookmark(_postId))
            .thenAnswer((_) async => const Err(NetworkFailure()));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const BookmarkRequested()),
      expect: () => [
        const BookmarkLoading(),
        isA<BookmarkError>().having(
          (s) => s.failure,
          'failure',
          isA<NetworkFailure>(),
        ),
      ],
    );

    // -----------------------------------------------------------------------
    // Unbookmark failure
    // -----------------------------------------------------------------------

    blocTest<BookmarkToggleBloc, BookmarkToggleState>(
      'emits [BookmarkLoading, BookmarkError] when unbookmark returns a failure',
      build: () {
        when(() => mockRepo.unbookmark(_postId))
            .thenAnswer((_) async => const Err(ServerFailure()));
        return _makeBloc(mockRepo);
      },
      act: (bloc) => bloc.add(const UnbookmarkRequested()),
      expect: () => [
        const BookmarkLoading(),
        isA<BookmarkError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
    );
  });
}
