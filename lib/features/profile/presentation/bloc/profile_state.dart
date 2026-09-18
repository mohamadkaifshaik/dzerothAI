part of 'profile_bloc.dart';

/// All states that [ProfileBloc] can emit.
sealed class ProfileState extends Equatable {
  const ProfileState();

  @override
  List<Object?> get props => [];
}

/// No profile has been requested yet.
final class ProfileInitial extends ProfileState {
  const ProfileInitial();
}

/// A profile load is in progress.
final class ProfileLoading extends ProfileState {
  const ProfileLoading();
}

/// A profile was loaded successfully.
final class ProfileLoaded extends ProfileState {
  const ProfileLoaded({required this.profile});

  /// May be a [Profile] (public) or an [OwnProfile] (own profile).
  final Profile profile;

  @override
  List<Object?> get props => [profile];
}

/// Loading the profile failed.
final class ProfileError extends ProfileState {
  const ProfileError({required this.failure});

  final Failure failure;

  @override
  List<Object?> get props => [failure];
}

/// A profile update is being submitted.
final class EditProfileSubmitting extends ProfileState {
  const EditProfileSubmitting({required this.current});

  final OwnProfile current;

  @override
  List<Object?> get props => [current];
}

/// A profile update completed successfully.
final class EditProfileSuccess extends ProfileState {
  const EditProfileSuccess({required this.updated});

  final OwnProfile updated;

  @override
  List<Object?> get props => [updated];
}

/// A profile update failed.
final class EditProfileError extends ProfileState {
  const EditProfileError({required this.failure, required this.current});

  final Failure failure;
  final OwnProfile current;

  @override
  List<Object?> get props => [failure, current];
}
