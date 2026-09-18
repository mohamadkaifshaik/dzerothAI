// ignore_for_file: prefer_initializing_formals
import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/entities/own_profile.dart';
import '../../domain/entities/profile.dart';
import '../../domain/repositories/profile_repository.dart';

part 'profile_event.dart';
part 'profile_state.dart';

/// Manages profile loading and editing state.
///
/// A single instance handles both "view own profile" and "view other user's
/// profile" to avoid duplicate BLoC registration for the same feature.
class ProfileBloc extends Bloc<ProfileEvent, ProfileState> {
  ProfileBloc({required ProfileRepository profileRepository})
    : _profileRepository = profileRepository,
      super(const ProfileInitial()) {
    on<OwnProfileLoadRequested>(_onOwnProfileLoadRequested);
    on<ProfileLoadRequested>(_onProfileLoadRequested);
    on<ProfileUpdateRequested>(_onProfileUpdateRequested);
  }

  final ProfileRepository _profileRepository;

  Future<void> _onOwnProfileLoadRequested(
    OwnProfileLoadRequested event,
    Emitter<ProfileState> emit,
  ) async {
    emit(const ProfileLoading());
    final result = await _profileRepository.getOwnProfile();
    switch (result) {
      case Success(:final value):
        emit(ProfileLoaded(profile: value));
      case Err(:final failure):
        emit(ProfileError(failure: failure));
    }
  }

  Future<void> _onProfileLoadRequested(
    ProfileLoadRequested event,
    Emitter<ProfileState> emit,
  ) async {
    emit(const ProfileLoading());
    final result = await _profileRepository.getUserProfile(event.userId);
    switch (result) {
      case Success(:final value):
        emit(ProfileLoaded(profile: value));
      case Err(:final failure):
        emit(ProfileError(failure: failure));
    }
  }

  Future<void> _onProfileUpdateRequested(
    ProfileUpdateRequested event,
    Emitter<ProfileState> emit,
  ) async {
    // Guard: we can only update if we already have the own profile loaded.
    final currentState = state;
    final current = switch (currentState) {
      ProfileLoaded(:final profile) when profile is OwnProfile => profile,
      EditProfileError(:final current) => current,
      _ => null,
    };

    if (current == null) return;

    emit(EditProfileSubmitting(current: current));

    final result = await _profileRepository.updateOwnProfile(
      displayName: event.displayName,
      bio: event.bio,
      location: event.location,
      websiteUrl: event.websiteUrl,
    );

    switch (result) {
      case Success(:final value):
        emit(EditProfileSuccess(updated: value));
      case Err(:final failure):
        emit(EditProfileError(failure: failure, current: current));
    }
  }
}
