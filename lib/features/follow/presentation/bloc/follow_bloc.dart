// ignore_for_file: prefer_initializing_formals
import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/repositories/follow_repository.dart';

part 'follow_event.dart';
part 'follow_state.dart';

/// Manages the follow/unfollow toggle state for a single target user.
///
/// A new instance is created per profile view so state does not leak across
/// different users.
class FollowBloc extends Bloc<FollowEvent, FollowState> {
  FollowBloc({
    required FollowRepository followRepository,
    required String targetUserId,
  }) : _repository = followRepository,
       _targetUserId = targetUserId,
       super(const FollowInitial()) {
    on<FollowRequested>(_onFollowRequested);
    on<UnfollowRequested>(_onUnfollowRequested);
    on<FollowStatusCheckRequested>(_onFollowStatusCheckRequested);
  }

  final FollowRepository _repository;
  final String _targetUserId;

  Future<void> _onFollowRequested(
    FollowRequested event,
    Emitter<FollowState> emit,
  ) async {
    emit(const FollowLoading());
    final result = await _repository.follow(_targetUserId);
    switch (result) {
      case Success():
        emit(const FollowSuccess(isFollowing: true));
      case Err(:final failure):
        emit(FollowError(failure: failure));
    }
  }

  Future<void> _onUnfollowRequested(
    UnfollowRequested event,
    Emitter<FollowState> emit,
  ) async {
    emit(const FollowLoading());
    final result = await _repository.unfollow(_targetUserId);
    switch (result) {
      case Success():
        emit(const FollowSuccess(isFollowing: false));
      case Err(:final failure):
        emit(FollowError(failure: failure));
    }
  }

  Future<void> _onFollowStatusCheckRequested(
    FollowStatusCheckRequested event,
    Emitter<FollowState> emit,
  ) async {
    emit(const FollowLoading());
    final result = await _repository.isFollowing(_targetUserId);
    switch (result) {
      case Success(:final value):
        emit(FollowSuccess(isFollowing: value));
      case Err(:final failure):
        emit(FollowError(failure: failure));
    }
  }
}
