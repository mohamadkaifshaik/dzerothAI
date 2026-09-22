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

final _resultWithPrimary2 = MyTitlesResult(
  titles: [_title1, _title2],
  primaryId: 'id-2',
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
  // TestTitleLibraryBloc_SetPrimaryTitle — pessimistic mutation
  // ---------------------------------------------------------------------------

  group('TestTitleLibraryBloc_SetPrimaryTitle', () {
    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'Mutating state preserves OLD primaryId — does NOT show new id before API responds',
      build: () {
        // Use a call counter so the first getMyTitles call returns _resultWithPrimary
        // and the second (post-mutation) returns _resultWithPrimary2.
        var getMyTitlesCallCount = 0;
        when(() => mockRepo.getMyTitles()).thenAnswer((_) async {
          getMyTitlesCallCount++;
          return getMyTitlesCallCount == 1
              ? Success(_resultWithPrimary)
              : Success(_resultWithPrimary2);
        });
        when(() => mockRepo.setPrimaryTitle('id-2'))
            .thenAnswer((_) async => const Success('legend'));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const LoadTitleLibrary());
        await Future<void>.delayed(Duration.zero);
        bloc.add(const SetPrimaryTitle('id-2'));
      },
      expect: () => [
        const TitleLibraryLoading(),
        // Server-confirmed initial load — primaryId is 'id-1'.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        // Mutating: old primaryId 'id-1' is preserved — 'id-2' is NOT shown yet.
        TitleLibraryMutating(titles: [_title1, _title2], primaryId: 'id-1'),
        // Server-confirmed after getMyTitles: primaryId is now 'id-2'.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-2'),
      ],
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'SetPrimaryTitle success: calls setPrimaryTitle then getMyTitles; '
      'final state reflects server-returned primaryId',
      build: () {
        // First call: initial load (no primary); second call: post-mutation (id-1).
        var getMyTitlesCallCount = 0;
        when(() => mockRepo.getMyTitles()).thenAnswer((_) async {
          getMyTitlesCallCount++;
          return getMyTitlesCallCount == 1
              ? Success(_resultNoPrimary)
              : Success(_resultWithPrimary);
        });
        when(() => mockRepo.setPrimaryTitle('id-1'))
            .thenAnswer((_) async => const Success('pioneer'));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const LoadTitleLibrary());
        await Future<void>.delayed(Duration.zero);
        bloc.add(const SetPrimaryTitle('id-1'));
      },
      expect: () => [
        const TitleLibraryLoading(),
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: null),
        // Mutating — old primaryId null preserved.
        TitleLibraryMutating(titles: [_title1, _title2], primaryId: null),
        // Server confirmed primaryId: 'id-1'.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
      ],
      verify: (_) {
        verify(() => mockRepo.setPrimaryTitle('id-1')).called(1);
        // getMyTitles called twice: once for initial load, once after mutation.
        verify(() => mockRepo.getMyTitles()).called(2);
      },
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'SetPrimaryTitle failure: previous primaryId preserved; error state emitted',
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
        // Loaded with original primary 'id-1'.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        // Mutating — old primaryId 'id-1' preserved.
        TitleLibraryMutating(titles: [_title1, _title2], primaryId: 'id-1'),
        // Restored to prior confirmed state on failure — 'id-1' not 'id-2'.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        // Error state surfaced.
        isA<TitleLibraryError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
      verify: (_) {
        verify(() => mockRepo.setPrimaryTitle('id-2')).called(1);
        // getMyTitles NOT called after a failed setPrimaryTitle.
        verify(() => mockRepo.getMyTitles()).called(1);
      },
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'SetPrimaryTitle succeeds but getMyTitles fails: '
      'error emitted, no fabricated local primaryId',
      build: () {
        // First call: initial load succeeds; second call (post-mutation) fails.
        var getMyTitlesCallCount = 0;
        when(() => mockRepo.getMyTitles()).thenAnswer((_) async {
          getMyTitlesCallCount++;
          return getMyTitlesCallCount == 1
              ? Success(_resultWithPrimary)
              : const Err(NetworkFailure());
        });
        when(() => mockRepo.setPrimaryTitle('id-2'))
            .thenAnswer((_) async => const Success('legend'));
        return TitleLibraryBloc(titleRepository: mockRepo);
      },
      act: (bloc) async {
        bloc.add(const LoadTitleLibrary());
        await Future<void>.delayed(Duration.zero);
        bloc.add(const SetPrimaryTitle('id-2'));
      },
      expect: () => [
        const TitleLibraryLoading(),
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        TitleLibraryMutating(titles: [_title1, _title2], primaryId: 'id-1'),
        // Error emitted — no fabricated local primaryId 'id-2' in any state.
        isA<TitleLibraryError>().having(
          (s) => s.failure,
          'failure',
          isA<NetworkFailure>(),
        ),
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
  // TestTitleLibraryBloc_ClearPrimaryTitle — pessimistic mutation
  // ---------------------------------------------------------------------------

  group('TestTitleLibraryBloc_ClearPrimaryTitle', () {
    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'ClearPrimaryTitle success: calls clearPrimaryTitle then getMyTitles; '
      'final state reflects server-confirmed null primaryId',
      build: () {
        // First call: initial load with primary 'id-1'; second call (post-clear) no primary.
        var getMyTitlesCallCount = 0;
        when(() => mockRepo.getMyTitles()).thenAnswer((_) async {
          getMyTitlesCallCount++;
          return getMyTitlesCallCount == 1
              ? Success(_resultWithPrimary)
              : Success(_resultNoPrimary);
        });
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
        // Loaded with primary 'id-1'.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        // Mutating — old primaryId 'id-1' preserved; NOT cleared yet.
        TitleLibraryMutating(titles: [_title1, _title2], primaryId: 'id-1'),
        // Server-confirmed: null primaryId.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: null),
      ],
      verify: (_) {
        verify(() => mockRepo.clearPrimaryTitle()).called(1);
        verify(() => mockRepo.getMyTitles()).called(2);
      },
    );

    blocTest<TitleLibraryBloc, TitleLibraryState>(
      'ClearPrimaryTitle failure: previous primaryId preserved; error emitted',
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
        // Mutating — old primaryId 'id-1' preserved.
        TitleLibraryMutating(titles: [_title1, _title2], primaryId: 'id-1'),
        // Restored to prior confirmed state — primaryId still 'id-1'.
        TitleLibraryLoaded(titles: [_title1, _title2], primaryId: 'id-1'),
        isA<TitleLibraryError>(),
      ],
      verify: (_) {
        verify(() => mockRepo.clearPrimaryTitle()).called(1);
        // getMyTitles NOT called after a failed clearPrimaryTitle.
        verify(() => mockRepo.getMyTitles()).called(1);
      },
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
