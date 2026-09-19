import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/settings/domain/repositories/settings_repository.dart';
import 'package:dzeroth/features/settings/presentation/bloc/settings_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockSettingsRepository extends Mock implements SettingsRepository {}

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

const _publicSettings = SettingsData(isPrivate: false);
const _privateSettings = SettingsData(isPrivate: true);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockSettingsRepository mockRepo;

  setUp(() {
    mockRepo = MockSettingsRepository();
  });

  // TestSettingsBloc_FetchLoaded
  group('SettingsFetchRequested', () {
    blocTest<SettingsBloc, SettingsState>(
      'TestSettingsBloc_FetchLoaded: emits [SettingsLoading, SettingsLoaded] on success',
      build: () {
        when(() => mockRepo.getSettings())
            .thenAnswer((_) async => const Success(_publicSettings));
        return SettingsBloc(settingsRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const SettingsFetchRequested()),
      expect: () => [
        const SettingsLoading(),
        const SettingsLoaded(isPrivate: false),
      ],
    );

    blocTest<SettingsBloc, SettingsState>(
      'emits [SettingsLoading, SettingsError] when fetch fails',
      build: () {
        when(() => mockRepo.getSettings())
            .thenAnswer((_) async => const Err(ServerFailure('fetch failed')));
        return SettingsBloc(settingsRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const SettingsFetchRequested()),
      expect: () => [
        const SettingsLoading(),
        isA<SettingsError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
    );
  });

  // TestSettingsBloc_TogglePrivacy_EmitsSaved
  group('SettingsPrivacyToggled', () {
    blocTest<SettingsBloc, SettingsState>(
      'TestSettingsBloc_TogglePrivacy_EmitsSaved: emits [SettingsSaving, SettingsSaved] on success',
      build: () {
        when(() => mockRepo.updatePrivacy(isPrivate: any(named: 'isPrivate')))
            .thenAnswer((_) async => const Success(_privateSettings));
        return SettingsBloc(settingsRepository: mockRepo);
      },
      seed: () => const SettingsLoaded(isPrivate: false),
      act: (bloc) => bloc.add(const SettingsPrivacyToggled(isPrivate: true)),
      expect: () => [
        const SettingsSaving(isPrivate: true),
        const SettingsSaved(isPrivate: true),
      ],
    );

    // TestSettingsBloc_SaveError_EmitsError
    blocTest<SettingsBloc, SettingsState>(
      'TestSettingsBloc_SaveError_EmitsError: emits [SettingsSaving, SettingsError, SettingsLoaded(rollback)] on failure',
      build: () {
        when(() => mockRepo.updatePrivacy(isPrivate: any(named: 'isPrivate')))
            .thenAnswer((_) async => const Err(ServerFailure('update failed')));
        return SettingsBloc(settingsRepository: mockRepo);
      },
      // Seed with isPrivate: false — the previous known value.
      seed: () => const SettingsLoaded(isPrivate: false),
      act: (bloc) => bloc.add(const SettingsPrivacyToggled(isPrivate: true)),
      expect: () => [
        const SettingsSaving(isPrivate: true),
        isA<SettingsError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
        // Rollback to previous value.
        const SettingsLoaded(isPrivate: false),
      ],
    );
  });

  // TestSettingsBloc_AccountSuspension_EmitsSuspended
  group('SettingsAccountSuspensionRequested', () {
    blocTest<SettingsBloc, SettingsState>(
      'TestSettingsBloc_AccountSuspension_EmitsSuspended: emits [SettingsLoading, SettingsAccountSuspended] on success',
      build: () {
        when(() => mockRepo.suspendAccount())
            .thenAnswer((_) async => const Success(null));
        return SettingsBloc(settingsRepository: mockRepo);
      },
      seed: () => const SettingsLoaded(isPrivate: false),
      act: (bloc) => bloc.add(const SettingsAccountSuspensionRequested()),
      expect: () => [const SettingsLoading(), const SettingsAccountSuspended()],
    );

    blocTest<SettingsBloc, SettingsState>(
      'emits [SettingsLoading, SettingsError] when suspension fails',
      build: () {
        when(() => mockRepo.suspendAccount()).thenAnswer(
          (_) async => const Err(ServerFailure('suspension failed')),
        );
        return SettingsBloc(settingsRepository: mockRepo);
      },
      seed: () => const SettingsLoaded(isPrivate: false),
      act: (bloc) => bloc.add(const SettingsAccountSuspensionRequested()),
      expect: () => [
        const SettingsLoading(),
        isA<SettingsError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
    );
  });
}
