// ignore_for_file: prefer_initializing_formals
import 'package:equatable/equatable.dart';
import 'package:flutter_bloc/flutter_bloc.dart';

import '../../../../core/error/failures.dart';
import '../../../../core/error/result.dart';
import '../../domain/repositories/block_repository.dart';

part 'block_event.dart';
part 'block_state.dart';

/// Manages block/unblock and mute/unmute actions for a single target user.
///
/// A new instance is created per profile view so state does not leak across
/// different users.
class BlockBloc extends Bloc<BlockEvent, BlockState> {
  BlockBloc({
    required BlockRepository blockRepository,
    required String targetUserId,
  }) : _repository = blockRepository,
       _targetUserId = targetUserId,
       super(const BlockInitial()) {
    on<BlockUserRequested>(_onBlockUserRequested);
    on<UnblockUserRequested>(_onUnblockUserRequested);
    on<MuteUserRequested>(_onMuteUserRequested);
    on<UnmuteUserRequested>(_onUnmuteUserRequested);
  }

  final BlockRepository _repository;
  final String _targetUserId;

  Future<void> _onBlockUserRequested(
    BlockUserRequested event,
    Emitter<BlockState> emit,
  ) async {
    emit(const BlockLoading());
    final result = await _repository.blockUser(_targetUserId);
    switch (result) {
      case Success():
        emit(const BlockSuccess(action: 'blocked'));
      case Err(:final failure):
        emit(BlockError(failure: failure));
    }
  }

  Future<void> _onUnblockUserRequested(
    UnblockUserRequested event,
    Emitter<BlockState> emit,
  ) async {
    emit(const BlockLoading());
    final result = await _repository.unblockUser(_targetUserId);
    switch (result) {
      case Success():
        emit(const BlockSuccess(action: 'unblocked'));
      case Err(:final failure):
        emit(BlockError(failure: failure));
    }
  }

  Future<void> _onMuteUserRequested(
    MuteUserRequested event,
    Emitter<BlockState> emit,
  ) async {
    emit(const BlockLoading());
    final result = await _repository.muteUser(_targetUserId);
    switch (result) {
      case Success():
        emit(const BlockSuccess(action: 'muted'));
      case Err(:final failure):
        emit(BlockError(failure: failure));
    }
  }

  Future<void> _onUnmuteUserRequested(
    UnmuteUserRequested event,
    Emitter<BlockState> emit,
  ) async {
    emit(const BlockLoading());
    final result = await _repository.unmuteUser(_targetUserId);
    switch (result) {
      case Success():
        emit(const BlockSuccess(action: 'unmuted'));
      case Err(:final failure):
        emit(BlockError(failure: failure));
    }
  }
}
