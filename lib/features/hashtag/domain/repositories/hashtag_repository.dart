import '../../../../core/error/result.dart';
import '../../../post/domain/entities/post.dart';

/// Abstract repository for hashtag feed data.
abstract class HashtagRepository {
  /// Fetches a cursor-paginated page of posts tagged with [tag].
  ///
  /// [tag] may include a leading '#' — the implementation normalizes it.
  /// [cursor] is the opaque pagination cursor returned by the previous call.
  /// Returns [PostPage] or a [Failure].
  Future<Result<PostPage>> getHashtagFeed({
    required String tag,
    String? cursor,
  });
}
