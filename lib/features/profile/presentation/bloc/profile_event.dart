part of 'profile_bloc.dart';

/// All events that can be dispatched to [ProfileBloc].
sealed class ProfileEvent {
  const ProfileEvent();
}

/// Load the authenticated user's own profile.
final class OwnProfileLoadRequested extends ProfileEvent {
  const OwnProfileLoadRequested();
}

/// Load a public profile by user ID.
final class ProfileLoadRequested extends ProfileEvent {
  const ProfileLoadRequested({required this.userId});

  final String userId;
}

/// Submit a profile update for the authenticated user.
final class ProfileUpdateRequested extends ProfileEvent {
  const ProfileUpdateRequested({
    this.displayName,
    this.bio,
    this.location,
    this.websiteUrl,
  });

  final String? displayName;
  final String? bio;
  final String? location;
  final String? websiteUrl;
}
