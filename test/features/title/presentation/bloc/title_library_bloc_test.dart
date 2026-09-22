import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/title/domain/entities/user_title.dart';
import 'package:dzeroth/features/title/domain/repositories/title_repository.dart';
import 'package:dzeroth/features/title/presentation/bloc/title_library_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockTitleRepository extends Mock implements TitleRepository {}

// ---------------------------------------------------------------------------
// Fixtures
// ---------------------------------------------------------------------------

final _dt = DateTime(2026, 1, 1);

UserTitle _titleFixture(String id, String slug) => UserTitle(
  id: id,
  slug: slug,
  displayName: slug,
  category: 'test',
  isRevocable: false,
  status: 'active',
  unlockedAt: _dt,
);

final _title1 = _titleFixture('id-1', 'pioneer');
final _title2 = _titleFixture('id-2', 'legend');

final _resultWithPrimary = MyTitlesResult(
  titles: [_title1, _title2],
  primaryId: 'id-1',
);

final _resultNoPrimary = MyTitlesResult(
  titles: [_title1, _title2],
  primaryId: null,
);

const _resultEmpty = MyTitlesResult(titles: [], primaryId: null);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockTitleRepository mockRepo;

  setUp(() {
    mockRepo = MockTitleRepository();
  });

  // ---------------------------------------------------------------------------
  // TestTitleLibraryBloc_LoadTitleLibrary_Loaded
  // ---------------------------------------------------------------------------

  group('TestTitleLibraryBloc_LoadTitleLibrary_Loaded', () {
    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'emits [Loading, Loaded] when repository returns titles with primaryId',
      build: () {
        when(() => mockRepo.getMyTitles())
            .thenAnswer((_) async => Success(_resultWithPrimary));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const LoadTitleLibrary()),
      expect: () => [
        const TitleLibraryLoading(),
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
      ],
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'emits [Loading, Loaded(primaryId: null)] when no primary is set',
      build: () {
        when(() => mockRepo.getMyTitles())
            .thenAnswer((_) async => Success(_resultNoPrimary));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const LoadTitleLibrary()),
      expect: () => [
        const TitleLibraryLoading(),
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: null),
      ],
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'emits [Loading, Loaded(titles: [])] when user has no titles',
      build: () {
        when(() => mockRepo.getMyTitles())
            .thenAnswer((_) async => const Success(_resultEmpty));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const LoadTitleLibrary()),
      expect: () => [
        const TitleLibraryLoading(),
        const TitleLibraryLoaded(titles: [], primaryId: null),
      ],
    );
  });

  // ---------------------------------------------------------------------------
  // TestTitleLibraryBloc_LoadTitleLibrary_Error
  // ---------------------------------------------------------------------------

  group('TestTitleLibraryBloc_LoadTitleLibrary_Error', () {
    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'emits [Loading, Error] when repository returns ServerFailure',
      build: () {
        when(() => mockRepo.getMyTitles())
            .thenAnswer((_) async => const Err(ServerFailure('Exploded.')));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const LoadTitleLibrary()),
      expect: () => [
        const TitleLibraryLoading(),
        isA<TitleLibraryError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'emits [Loading, Error] when repository returns NetworkFailure',
      build: () {
        when(() => mockRepo.getMyTitles())
            .thenAnswer((_) async => const Err(NetworkFailure()));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const LoadTitleLibrary()),
      expect: () => [
        const TitleLibraryLoading(),
        isA<TitleLibraryError>().having(
          (s) => s.failure,
          'failure',
          isA<NetworkFailure>(),
        ),
      ],
    );
  });

  // ---------------------------------------------------------------------------
  // TestTitleLibraryBloc_SetPrimaryTitle
  // ---------------------------------------------------------------------------

  group('TestTitleLibraryBloc_SetPrimaryTitle', () {
    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'optimistically sets primaryId on success',
      build: () {
        when(() => mockRepo.getMyTitles())
            .thenAnswer((_) async => Success(_resultNoPrimary));
        when(() => mockRepo.setPrimaryTitle('id-2'))
            .thenAnswer((_) async => const Success('legend'));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const LoadTitleLibrary());
        await Future<void>.delayed(Duration.zero);
        bloc.add(const SetPrimaryTitle('id-2'));
      },
      // BLoC deduplicate: optimistic emit and confirmed emit are identical
      // states — the second emit is a no-op because the state is unchanged.
      // Equatable equality means only 3 distinct emissions occur.
      expect: () => [
        const TitleLibraryLoading(),
        // Initial load — no primary.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: null),
        // Optimistic update (confirmed emit is deduplicated by Equatable).
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-2'),
      ],
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'reverts to previous primaryId when setPrimaryTitle fails',
      build: () {
        when(() => mockRepo.getMyTitles())
            .thenAnswer((_) async => Success(_resultWithPrimary));
        when(() => mockRepo.setPrimaryTitle('id-2'))
            .thenAnswer((_) async => const Err(ServerFailure('Failed.')));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const LoadTitleLibrary());
        await Future<void>.delayed(Duration.zero);
        bloc.add(const SetPrimaryTitle('id-2'));
      },
      expect: () => [
        const TitleLibraryLoading(),
        // Loaded with original primary.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        // Optimistic: primaryId switched to id-2.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-2'),
        // Reverted on failure.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        // Error state after revert.
        isA<TitleLibraryError>(),
      ],
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'ignores SetPrimaryTitle when not in Loaded state',
      build: () => TitleLibraryBloc(titleRepository: mockRepo),
      act: (bloc) => bloc.add(const SetPrimaryTitle('id-1')),
      // Initial state — SetPrimaryTitle is silently dropped.
      expect: () => [],
      verify: (_) => verifyNever(() => mockRepo.setPrimaryTitle(any())),
    );
  });

  // ---------------------------------------------------------------------------
  // TestTitleLibraryBloc_ClearPrimaryTitle
  // ---------------------------------------------------------------------------

  group('TestTitleLibraryBloc_ClearPrimaryTitle', () {
    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'optimistically clears primaryId then confirms on success',
      build: () {
        when(() => mockRepo.getMyTitles())
            .thenAnswer((_) async => Success(_resultWithPrimary));
        when(() => mockRepo.clearPrimaryTitle())
            .thenAnswer((_) async => const Success(null));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const LoadTitleLibrary());
        await Future<void>.delayed(Duration.zero);
        bloc.add(const ClearPrimaryTitle());
      },
      expect: () => [
        const TitleLibraryLoading(),
        // Loaded with primary.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        // Optimistic clear.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: null),
      ],
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'reverts to previous primaryId when clearPrimaryTitle fails',
      build: () {
        when(() => mockRepo.getMyTitles())
            .thenAnswer((_) async => Success(_resultWithPrimary));
        when(() => mockRepo.clearPrimaryTitle())
            .thenAnswer((_) async => const Err(ServerFailure('Failed.')));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const LoadTitleLibrary());
        await Future<void>.delayed(Duration.zero);
        bloc.add(const ClearPrimaryTitle());
      },
      expect: () => [
        const TitleLibraryLoading(),
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        // Optimistic clear.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: null),
        // Reverted.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        isA<TitleLibraryError>(),
      ],
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'ignores ClearPrimaryTitle when not in Loaded state',
      build: () => TitleLibraryBloc(titleRepository: mockRepo),
      act: (bloc) => bloc.add(const ClearPrimaryTitle()),
      expect: () => [],
      verify: (_) => verifyNever(() => mockRepo.clearPrimaryTitle()),
    );
  });
}
