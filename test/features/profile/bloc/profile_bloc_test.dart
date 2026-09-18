import 'package:bloc_test/bloc_test.dart';
import 'package:flutter_test/flutter_test.dart';
import 'package:mocktail/mocktail.dart';

import 'package:dzeroth/core/error/failures.dart';
import 'package:dzeroth/core/error/result.dart';
import 'package:dzeroth/features/profile/domain/entities/own_profile.dart';
import 'package:dzeroth/features/profile/domain/entities/profile.dart';
import 'package:dzeroth/features/profile/domain/repositories/profile_repository.dart';
import 'package:dzeroth/features/profile/presentation/bloc/profile_bloc.dart';

// ---------------------------------------------------------------------------
// Mock
// ---------------------------------------------------------------------------

class MockProfileRepository extends Mock implements ProfileRepository {}

// ---------------------------------------------------------------------------
// Fixture data
// ---------------------------------------------------------------------------

const _testOwnProfile = OwnProfile(
  id: '01900000-0000-7000-8000-000000000010',
  handle: 'currentuser',
  displayName: 'Current User',
  bio: 'My bio',
  isPrivate: false,
  joinedAt: '2024-01-15T12:00:00Z',
  email: 'current@example.com',
  emailVerified: true,
);

const _testPublicProfile = Profile(
  id: '01900000-0000-7000-8000-000000000020',
  handle: 'otheruser',
  displayName: 'Other User',
  isPrivate: false,
  joinedAt: '2024-02-10T08:00:00Z',
);

const _updatedOwnProfile = OwnProfile(
  id: '01900000-0000-7000-8000-000000000010',
  handle: 'currentuser',
  displayName: 'Updated Name',
  bio: 'Updated bio',
  isPrivate: false,
  joinedAt: '2024-01-15T12:00:00Z',
  email: 'current@example.com',
  emailVerified: true,
);

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

void main() {
  late MockProfileRepository mockRepo;

  setUp(() {
    mockRepo = MockProfileRepository();
  });

  group('OwnProfileLoadRequested', () {
    blocTest<ProfileBloc, ProfileState>(
      'emits [ProfileLoading, ProfileLoaded(ownProfile)] on success',
      build: () {
        when(() => mockRepo.getOwnProfile()).thenAnswer(
          (_) async => const Success(_testOwnProfile),
        );
        return ProfileBloc(profileRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const OwnProfileLoadRequested()),
      expect: () => [
        const ProfileLoading(),
        const ProfileLoaded(profile: _testOwnProfile),
      ],
    );

    blocTest<ProfileBloc, ProfileState>(
      'emits [ProfileLoading, ProfileError(failure)] on failure',
      build: () {
        when(() => mockRepo.getOwnProfile()).thenAnswer(
          (_) async => const Err(ServerFailure('server error')),
        );
        return ProfileBloc(profileRepository: mockRepo);
      },
      act: (bloc) => bloc.add(const OwnProfileLoadRequested()),
      expect: () => [
        const ProfileLoading(),
        isA<ProfileError>().having(
          (s) => s.failure,
          'failure',
          isA<ServerFailure>(),
        ),
      ],
    );
  });

  group('ProfileLoadRequested', () {
    blocTest<ProfileBloc, ProfileState>(
      'emits [ProfileLoading, ProfileLoaded(publicProfile)] on success',
      build: () {
        when(
          () => mockRepo.getUserProfile(any()),
        ).thenAnswer((_) async => const Success(_testPublicProfile));
        return ProfileBloc(profileRepository: mockRepo);
      },
      act: (bloc) => bloc.add(
        const ProfileLoadRequested(
          userId: '01900000-0000-7000-8000-000000000020',
        ),
      ),
      expect: () => [
        const ProfileLoading(),
        const ProfileLoaded(profile: _testPublicProfile),
      ],
    );

    blocTest<ProfileBloc, ProfileState>(
      'emits [ProfileLoading, ProfileError] when user is not found',
      build: () {
        when(
          () => mockRepo.getUserProfile(any()),
        ).thenAnswer(
          (_) async => const Err(NotFoundFailure('User not found.')),
        );
        return ProfileBloc(profileRepository: mockRepo);
      },
      act: (bloc) => bloc.add(
        const ProfileLoadRequested(userId: 'nonexistent-id'),
      ),
      expect: () => [
        const ProfileLoading(),
        isA<ProfileError>().having(
          (s) => s.failure,
          'failure',
          isA<NotFoundFailure>(),
        ),
      ],
    );
  });

  group('ProfileUpdateRequested', () {
    blocTest<ProfileBloc, ProfileState>(
      'emits [EditProfileSubmitting, EditProfileSuccess] on success',
      build: () {
        when(
          () => mockRepo.updateOwnProfile(
            displayName: any(named: 'displayName'),
            bio: any(named: 'bio'),
            location: any(named: 'location'),
            websiteUrl: any(named: 'websiteUrl'),
          ),
        ).thenAnswer((_) async => const Success(_updatedOwnProfile));

        return ProfileBloc(profileRepository: mockRepo);
      },
      // Seed the BLoC with a loaded OwnProfile state so ProfileUpdateRequested
      // has a current profile to work with.
      seed: () => const ProfileLoaded(profile: _testOwnProfile),
      act: (bloc) => bloc.add(
        const ProfileUpdateRequested(
          displayName: 'Updated Name',
          bio: 'Updated bio',
        ),
      ),
      expect: () => [
        const EditProfileSubmitting(current: _testOwnProfile),
        const EditProfileSuccess(updated: _updatedOwnProfile),
      ],
    );

    blocTest<ProfileBloc, ProfileState>(
      'emits [EditProfileSubmitting, EditProfileError] on failure',
      build: () {
        when(
          () => mockRepo.updateOwnProfile(
            displayName: any(named: 'displayName'),
            bio: any(named: 'bio'),
            location: any(named: 'location'),
            websiteUrl: any(named: 'websiteUrl'),
          ),
        ).thenAnswer(
          (_) async => const Err(ServerFailure('update failed')),
        );

        return ProfileBloc(profileRepository: mockRepo);
      },
      seed: () => const ProfileLoaded(profile: _testOwnProfile),
      act: (bloc) => bloc.add(
        const ProfileUpdateRequested(displayName: 'Bad Update'),
      ),
      expect: () => [
        const EditProfileSubmitting(current: _testOwnProfile),
        isA<EditProfileError>()
            .having((s) => s.failure, 'failure', isA<ServerFailure>())
            .having((s) => s.current, 'current', _testOwnProfile),
      ],
    );

    blocTest<ProfileBloc, ProfileState>(
      'does nothing when no OwnProfile is loaded (guard condition)',
      build: () => ProfileBloc(profileRepository: mockRepo),
      // No seed — starts at ProfileInitial.
      act: (bloc) => bloc.add(
        const ProfileUpdateRequested(displayName: 'Should not emit'),
      ),
      expect: () => <ProfileState>[],
    );
  });

  group('ProfileLoaded — metric lockdown', () {
    test(
        'Profile entity has no follower/like/impression/bookmark count fields',
        () {
      // Confirm via reflection that the Profile entity does not carry
      // any social-validation metric fields.
      //
      // We iterate the props list returned by Profile (which via Equatable
      // represents the canonical field list) and check our fixture.
      //
      // The authoritative invariant check is in the Go model_test.go via
      // reflection on JSON tags, but we also verify the Dart entity shape
      // does not inadvertently carry such fields.
      final props = _testPublicProfile.props;

      // The props for Profile are:
      // [id, handle, displayName, bio, avatarUrl, headerUrl,
      //  location, websiteUrl, isPrivate, joinedAt]
      // There must be exactly 10 props — no metric extras.
      expect(props.length, 10,
          reason:
              'Profile.props must have exactly 10 entries — no social-validation metrics');
    });

    test('OwnProfile entity extends Profile props with only email + emailVerified',
        () {
      final ownProps = _testOwnProfile.props;
      final publicProps = _testPublicProfile.props;
      // OwnProfile.props = public props (10) + email + emailVerified = 12.
      expect(
        ownProps.length,
        publicProps.length + 2,
        reason:
            'OwnProfile.props must be Profile.props + 2 (email, emailVerified) — no metric extras',
      );
    });
  });
}
