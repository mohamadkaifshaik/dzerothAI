import '../../../../core/error/result.dart';
import '../entities/post.dart';

/// Contract for all post-related data operations.
///
/// Implementations live in the data layer.
abstract class PostRepository {
  /// Creates a new post.
  ///
  /// [postType] must be one of 'original', 'reply', 'quote', 'repost'.
  /// [content] is required unless [postType] is 'repost'.
  /// The five-second friction rule and the five-distinct-word quote rule are
  /// enforced by the backend; the client validates them as a UX guard only.
  ///
  /// [shareInitiatedAt] is required for 'repost' and 'quote' post types. It
  /// must be the UTC timestamp at which the mandatory 5-second countdown began.
  /// The backend verifies that at least 5 seconds elapsed between this value
  /// and the time of request receipt (CLAUDE.md §2.2 — backend is authoritative).
  /// Must be null for 'original' and 'reply' post types.
  Future<Result<Post>> createPost({
    required String postType,
    String? content,
    String? parentId,
    String? quotedPostId,
    DateTime? shareInitiatedAt,
  });

  /// Fetches a single post by its ID.
  Future<Result<Post>> getPost(String postId);

  /// Returns a paginated page of posts by the given author.
  ///
  /// Pass [cursor] from the previous [PostPage.nextCursor] to fetch the next
  /// page.  When [PostPage.terminated] is true the BLoC must stop fetching.
  Future<Result<PostPage>> listPostsByAuthor(String authorId, {String? cursor});

  /// Returns a paginated page of replies for the given thread root post.
  Future<Result<PostPage>> listThreadReplies(
    String threadRootId, {
    String? cursor,
  });

  /// Soft-deletes the caller's post.
  Future<Result<void>> deletePost(String postId);
}
