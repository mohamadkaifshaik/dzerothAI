import 'dart:async';

import 'package:flutter/widgets.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:go_router/go_router.dart';

import '../../features/auth/presentation/bloc/auth_bloc.dart';
import '../../features/auth/presentation/screens/login_screen.dart';
import '../../features/auth/presentation/screens/register_screen.dart';
import '../../features/block/domain/repositories/block_repository.dart';
import '../../features/block/presentation/bloc/block_bloc.dart';
import '../../features/bookmark/domain/repositories/bookmark_repository.dart';
import '../../features/bookmark/presentation/bloc/bookmark_list_bloc.dart';
import '../../features/bookmark/presentation/bloc/bookmark_toggle_bloc.dart';
import '../../features/bookmark/presentation/screens/bookmark_list_screen.dart';
import '../../features/feed/data/repositories/feed_repository_impl.dart';
import '../../features/feed/presentation/bloc/home_feed_bloc.dart';
import '../../features/feed/presentation/screens/home_feed_screen.dart';
import '../../features/follow/domain/repositories/follow_repository.dart';
import '../../features/follow/presentation/bloc/follow_bloc.dart';
import '../../features/notification/domain/repositories/notification_repository.dart';
import '../../features/notification/presentation/bloc/notification_list_bloc.dart';
import '../../features/notification/presentation/screens/notification_list_screen.dart';
import '../../features/post/domain/repositories/post_repository.dart';
import '../../features/post/presentation/bloc/post_compose_bloc.dart';
import '../../features/post/presentation/bloc/post_detail_bloc.dart';
import '../../features/post/presentation/bloc/post_feed_bloc.dart';
import '../../features/post/presentation/screens/post_compose_screen.dart';
import '../../features/post/presentation/screens/post_detail_screen.dart';
import '../../features/profile/domain/repositories/profile_repository.dart';
import '../../features/profile/presentation/bloc/profile_bloc.dart';
import '../../features/profile/presentation/screens/edit_profile_screen.dart';
import '../../features/profile/presentation/screens/profile_screen.dart';
import '../../features/reaction/domain/repositories/reaction_repository.dart';
import '../../features/reaction/presentation/bloc/reaction_toggle_bloc.dart';
import '../../features/report/domain/repositories/report_repository.dart';
import '../../features/search/data/repositories/search_repository_impl.dart';
import '../../features/search/presentation/bloc/search_bloc.dart';
import '../../features/search/presentation/screens/search_screen.dart';
import '../../features/settings/domain/repositories/settings_repository.dart';
import '../../features/settings/presentation/bloc/settings_bloc.dart';
import '../../features/settings/presentation/screens/settings_screen.dart';
import '../../features/shell/presentation/screens/app_shell.dart';
import '../../features/studio/domain/repositories/studio_repository.dart';
import '../../features/studio/presentation/bloc/studio_bloc.dart';
import '../../features/studio/presentation/screens/studio_screen.dart';

/// A [ChangeNotifier] that listens to an [AuthBloc] stream and notifies
/// go_router's [refreshListenable] when auth state changes.
class GoRouterRefreshStream extends ChangeNotifier {
  GoRouterRefreshStream(Stream<AuthState> stream) {
    _subscription = stream.listen((_) => notifyListeners());
  }

  late final StreamSubscription<AuthState> _subscription;

  @override
  void dispose() {
    _subscription.cancel();
    super.dispose();
  }
}

/// Builds and returns the application [GoRouter].
///
/// [authBloc] is required to wire the auth guard and [refreshListenable].
/// [profileRepositoryFactory] creates [ProfileRepository] instances for
/// screens that need them.
GoRouter createAppRouter({
  required AuthBloc authBloc,
  required ProfileRepository Function() profileRepositoryFactory,
  required PostRepository Function() postRepositoryFactory,
  required FeedRepositoryImpl Function() feedRepositoryFactory,
  required FollowRepository Function() followRepositoryFactory,
  required BlockRepository Function() blockRepositoryFactory,
  required BookmarkRepository Function() bookmarkRepositoryFactory,
  required ReactionRepository Function() reactionRepositoryFactory,
  required NotificationRepository Function() notificationRepositoryFactory,
  required SearchRepositoryImpl Function() searchRepositoryFactory,
  required StudioRepository Function() studioRepositoryFactory,
  required SettingsRepository Function() settingsRepositoryFactory,
  required ReportRepository Function() reportRepositoryFactory,
}) {
  final refreshStream = GoRouterRefreshStream(authBloc.stream);

  return GoRouter(
    refreshListenable: refreshStream,
    initialLocation: '/home/feed',
    redirect: (context, state) {
      final authState = authBloc.state;
      final isAuthRoute = state.matchedLocation.startsWith('/auth');
      final isInitial = authState is AuthInitial || authState is AuthLoading;

      // While checking auth status on startup, show nothing (stay put).
      if (isInitial) return null;

      final isAuthenticated = authState is AuthAuthenticated;

      if (!isAuthenticated && !isAuthRoute) {
        return '/auth/login';
      }
      if (isAuthenticated && isAuthRoute) {
        return '/home/feed';
      }
      return null;
    },

    routes: [
      // Auth routes (unauthenticated)
      GoRoute(
        path: '/auth/login',
        builder: (context, state) => const LoginScreen(),
      ),
      GoRoute(
        path: '/auth/register',
        builder: (context, state) => const RegisterScreen(),
      ),

      // Authenticated shell — ReportRepository is provided here so all
      // PostCard widgets rendered within the shell (feed, search results,
      // bookmarks) can access it via context.read<ReportRepository>().
      ShellRoute(
        builder: (context, state, child) =>
            RepositoryProvider<ReportRepository>(
              create: (_) => reportRepositoryFactory(),
              child: AppShell(child: child),
            ),
        routes: [
          GoRoute(
            path: '/home/feed',
            builder: (context, state) => BlocProvider(
              create: (_) =>
                  HomeFeedBloc(feedRepository: feedRepositoryFactory())
                    ..add(const HomeFeedLoadRequested()),
              child: HomeFeedScreen(
                bookmarkRepositoryFactory: bookmarkRepositoryFactory,
              ),
            ),
          ),
          GoRoute(
            path: '/home/search',
            builder: (context, state) => BlocProvider(
              create: (_) =>
                  SearchBloc(searchRepository: searchRepositoryFactory()),
              child: const SearchScreen(),
            ),
          ),
          GoRoute(
            path: '/home/notifications',
            redirect: (context, state) {
              final authState = authBloc.state;
              if (authState is! AuthAuthenticated) return '/auth/login';
              return null;
            },
            builder: (context, state) => BlocProvider(
              create: (_) => NotificationListBloc(
                notificationRepository: notificationRepositoryFactory(),
              ),
              child: const NotificationListScreen(),
            ),
          ),
          // /home/profile redirects to the current user's profile page.
          GoRoute(
            path: '/home/profile',
            redirect: (context, state) {
              final authState = authBloc.state;
              if (authState is AuthAuthenticated) {
                return '/users/${authState.userId}';
              }
              return '/auth/login';
            },
          ),

          // Bookmarks — inside shell so the bottom nav bar is visible.
          GoRoute(
            path: '/bookmarks',
            redirect: (context, state) {
              final authState = authBloc.state;
              if (authState is! AuthAuthenticated) return '/auth/login';
              return null;
            },
            builder: (context, state) => BlocProvider(
              create: (_) => BookmarkListBloc(
                bookmarkRepository: bookmarkRepositoryFactory(),
              ),
              child: const BookmarkListScreen(),
            ),
          ),
        ],
      ),

      // User profile (outside shell so it can be pushed as a full page).
      // ReportRepository is provided here so ProfileScreen can open
      // ReportSheet.forUser via context.read<ReportRepository>().
      GoRoute(
        path: '/users/:id',
        builder: (context, state) {
          final userId = state.pathParameters['id']!;
          final authState = authBloc.state;
          final isOwn =
              authState is AuthAuthenticated && authState.userId == userId;

          return RepositoryProvider<ReportRepository>(
            create: (_) => reportRepositoryFactory(),
            child: MultiBlocProvider(
              providers: [
                BlocProvider(
                  create: (_) =>
                      ProfileBloc(profileRepository: profileRepositoryFactory())
                        ..add(
                          isOwn
                              ? const OwnProfileLoadRequested()
                              : ProfileLoadRequested(userId: userId),
                        ),
                ),
                // FollowBloc and BlockBloc are always created so that the
                // profile screen can conditionally show social actions without
                // needing to rebuild the provider tree.
                BlocProvider(
                  create: (_) => FollowBloc(
                    followRepository: followRepositoryFactory(),
                    targetUserId: userId,
                  ),
                ),
                BlocProvider(
                  create: (_) => BlockBloc(
                    blockRepository: blockRepositoryFactory(),
                    targetUserId: userId,
                  ),
                ),
              ],
              child: ProfileScreen(userId: userId, isOwnProfile: isOwn),
            ),
          );
        },
        routes: [
          GoRoute(
            path: 'edit',
            builder: (context, state) {
              final userId = state.pathParameters['id']!;
              final authState = authBloc.state;

              // Guard: only own profile can be edited.
              if (authState is! AuthAuthenticated ||
                  authState.userId != userId) {
                // Redirect handled via redirect callback below.
                return const SizedBox.shrink();
              }

              // Reuse the parent route's ProfileBloc via context.
              return const EditProfileScreen();
            },
            redirect: (context, state) {
              final userId = state.pathParameters['id']!;
              final authState = authBloc.state;
              if (authState is! AuthAuthenticated ||
                  authState.userId != userId) {
                return '/users/$userId';
              }
              return null;
            },
          ),
        ],
      ),

      // Settings (outside shell)
      GoRoute(
        path: '/settings',
        redirect: (context, state) {
          final authState = authBloc.state;
          if (authState is! AuthAuthenticated) return '/auth/login';
          return null;
        },
        builder: (context, state) => BlocProvider(
          create: (_) =>
              SettingsBloc(settingsRepository: settingsRepositoryFactory()),
          child: const SettingsScreen(),
        ),
      ),

      // Post compose — requires authentication; outside shell (full-page).
      GoRoute(
        path: '/posts/new',
        redirect: (context, state) {
          final authState = authBloc.state;
          if (authState is! AuthAuthenticated) return '/auth/login';
          return null;
        },
        builder: (context, state) {
          final postType = state.uri.queryParameters['type'] ?? 'original';
          final parentId = state.uri.queryParameters['parent_id'];
          final quotedPostId = state.uri.queryParameters['quoted_post_id'];
          return BlocProvider(
            create: (_) =>
                PostComposeBloc(postRepository: postRepositoryFactory()),
            child: PostComposeScreen(
              postType: postType,
              parentId: parentId,
              quotedPostId: quotedPostId,
            ),
          );
        },
      ),

      // Post detail — public; outside shell (full-page).
      // ReportRepository is provided here so PostCard renders within the
      // thread can open ReportSheet.forPost.
      GoRoute(
        path: '/posts/:postId',
        builder: (context, state) {
          final postId = state.pathParameters['postId']!;
          final postRepository = postRepositoryFactory();
          return RepositoryProvider<ReportRepository>(
            create: (_) => reportRepositoryFactory(),
            child: MultiBlocProvider(
              providers: [
                BlocProvider(
                  create: (_) =>
                      PostDetailBloc(postRepository: postRepository)
                        ..add(PostDetailLoadRequested(postId: postId)),
                ),
                BlocProvider(
                  create: (_) => PostFeedBloc(postRepository: postRepository)
                    ..add(
                      PostFeedLoadRequested(
                        subjectId: postId,
                        feedType: FeedType.threadReplies,
                      ),
                    ),
                ),
                BlocProvider(
                  create: (_) => BookmarkToggleBloc(
                    bookmarkRepository: bookmarkRepositoryFactory(),
                    postId: postId,
                  ),
                ),
                BlocProvider(
                  create: (_) => ReactionToggleBloc(
                    reactionRepository: reactionRepositoryFactory(),
                    postId: postId,
                    initiallyReacted: false,
                  ),
                ),
              ],
              child: PostDetailScreen(postId: postId),
            ),
          );
        },
      ),

      // Creator Studio — private analytics; requires auth; outside shell
      // (full-page push-navigation destination, NOT a shell tab).
      GoRoute(
        path: '/studio',
        redirect: (context, state) {
          final authState = authBloc.state;
          if (authState is! AuthAuthenticated) return '/auth/login';
          return null;
        },
        builder: (context, state) => BlocProvider(
          create: (_) =>
              StudioBloc(studioRepository: studioRepositoryFactory()),
          child: const StudioScreen(),
        ),
      ),
    ],
  );
}
