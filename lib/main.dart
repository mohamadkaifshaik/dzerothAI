import 'package:flutter/material.dart';
import 'package:flutter_bloc/flutter_bloc.dart';
import 'package:flutter_secure_storage/flutter_secure_storage.dart';

import 'core/design_system/app_theme.dart';
import 'core/network/api_client.dart';
import 'core/router/app_router.dart';
import 'core/storage/secure_storage.dart';
import 'features/auth/data/repositories/auth_repository_impl.dart';
import 'features/auth/presentation/bloc/auth_bloc.dart';
import 'features/block/data/repositories/block_repository_impl.dart';
import 'features/block/domain/repositories/block_repository.dart';
import 'features/bookmark/data/repositories/bookmark_repository_impl.dart';
import 'features/bookmark/domain/repositories/bookmark_repository.dart';
import 'features/notification/data/datasources/notification_remote_data_source.dart';
import 'features/notification/data/repositories/notification_repository_impl.dart';
import 'features/notification/domain/repositories/notification_repository.dart';
import 'features/reaction/data/repositories/reaction_repository_impl.dart';
import 'features/reaction/domain/repositories/reaction_repository.dart';
import 'features/report/data/repositories/report_repository_impl.dart';
import 'features/report/domain/repositories/report_repository.dart';
import 'features/search/data/repositories/search_repository_impl.dart';
import 'features/feed/data/repositories/feed_repository_impl.dart';
import 'features/follow/data/repositories/follow_repository_impl.dart';
import 'features/follow/domain/repositories/follow_repository.dart';
import 'features/post/data/repositories/post_repository_impl.dart';
import 'features/post/domain/repositories/post_repository.dart';
import 'features/profile/data/repositories/profile_repository_impl.dart';
import 'features/profile/domain/repositories/profile_repository.dart';
import 'features/settings/data/repositories/settings_repository_impl.dart';
import 'features/settings/domain/repositories/settings_repository.dart';
import 'features/studio/data/repositories/studio_repository_impl.dart';
import 'features/studio/domain/repositories/studio_repository.dart';

void main() async {
  WidgetsFlutterBinding.ensureInitialized();

  // --- Storage ---
  const flutterSecureStorage = FlutterSecureStorage(
    aOptions: AndroidOptions(encryptedSharedPreferences: true),
  );
  final secureStorage = SecureStorage(flutterSecureStorage);

  // --- Auth BLoC (created early so interceptor can reference its state) ---
  // We use a late-bound callback so the interceptor references the BLoC's
  // current access token without a circular dependency.
  String? currentAccessToken;
  void Function(String, String)? onTokenRefreshedCallback;
  void Function()? onUnauthenticatedCallback;

  // --- Dio / API client ---
  final dio = createApiClient(
    secureStorage: secureStorage,
    getAccessToken: () => currentAccessToken,
    onTokenRefreshed: (access, refresh) {
      currentAccessToken = access;
      onTokenRefreshedCallback?.call(access, refresh);
    },
    onUnauthenticated: () => onUnauthenticatedCallback?.call(),
  );

  // --- Auth repository ---
  final authRepository = AuthRepositoryImpl(
    dio: dio,
    secureStorage: secureStorage,
  );

  // --- Auth BLoC ---
  final authBloc = AuthBloc(authRepository: authRepository)
    ..add(const AuthCheckRequested());

  // Wire interceptor callbacks back to authBloc.
  onTokenRefreshedCallback = (access, refresh) {
    authBloc.add(
      AuthTokenRefreshed(accessToken: access, refreshToken: refresh),
    );
  };
  onUnauthenticatedCallback = () {
    authBloc.add(const AuthSessionExpired());
  };

  // Sync access token from BLoC state to interceptor reference.
  authBloc.stream.listen((state) {
    if (state is AuthAuthenticated) {
      currentAccessToken = state.accessToken;
    } else if (state is AuthUnauthenticated) {
      currentAccessToken = null;
    }
  });

  // --- Profile repository factory ---
  // Each ProfileBloc gets its own repository backed by the shared Dio instance.
  ProfileRepository profileRepositoryFactory() =>
      ProfileRepositoryImpl(dio: dio);

  // --- Post repository factory ---
  // Each screen that needs post data gets its own repository backed by the
  // shared authenticated Dio instance.
  PostRepository postRepositoryFactory() => PostRepositoryImpl(dio: dio);

  // --- Feed repository factory ---
  // Each HomeFeedBloc instance gets its own repository backed by the shared
  // authenticated Dio instance.
  FeedRepositoryImpl feedRepositoryFactory() => FeedRepositoryImpl(dio: dio);

  // --- Follow repository factory ---
  FollowRepository followRepositoryFactory() => FollowRepositoryImpl(dio: dio);

  // --- Block repository factory ---
  BlockRepository blockRepositoryFactory() => BlockRepositoryImpl(dio: dio);

  // --- Bookmark repository factory ---
  BookmarkRepository bookmarkRepositoryFactory() =>
      BookmarkRepositoryImpl(dio: dio);

  // --- Reaction repository factory ---
  ReactionRepository reactionRepositoryFactory() =>
      ReactionRepositoryImpl(dio: dio);

  // --- Notification repository factory ---
  NotificationRepository notificationRepositoryFactory() =>
      NotificationRepositoryImpl(
        dataSource: NotificationRemoteDataSource(dio: dio),
      );

  // --- Search repository factory ---
  // Auth is optional for search — the shared Dio instance attaches a token
  // when available but search endpoints also work unauthenticated.
  SearchRepositoryImpl searchRepositoryFactory() =>
      SearchRepositoryImpl(dio: dio);

  // --- Settings repository factory ---
  SettingsRepository settingsRepositoryFactory() =>
      SettingsRepositoryImpl(dio: dio);

  // --- Studio repository factory ---
  // Auth is required. The shared authenticated Dio instance is used.
  StudioRepository studioRepositoryFactory() => StudioRepositoryImpl(dio: dio);

  // --- Report repository factory ---
  // Auth is required — the shared authenticated Dio instance is used.
  ReportRepository reportRepositoryFactory() => ReportRepositoryImpl(dio: dio);

  // --- Router ---
  final router = createAppRouter(
    authBloc: authBloc,
    profileRepositoryFactory: profileRepositoryFactory,
    postRepositoryFactory: postRepositoryFactory,
    feedRepositoryFactory: feedRepositoryFactory,
    followRepositoryFactory: followRepositoryFactory,
    blockRepositoryFactory: blockRepositoryFactory,
    bookmarkRepositoryFactory: bookmarkRepositoryFactory,
    reactionRepositoryFactory: reactionRepositoryFactory,
    notificationRepositoryFactory: notificationRepositoryFactory,
    searchRepositoryFactory: searchRepositoryFactory,
    settingsRepositoryFactory: settingsRepositoryFactory,
    studioRepositoryFactory: studioRepositoryFactory,
    reportRepositoryFactory: reportRepositoryFactory,
  );

  runApp(DzerothApp(authBloc: authBloc, router: router));
}

/// Root widget for the Dzeroth application.
class DzerothApp extends StatelessWidget {
  const DzerothApp({super.key, required this.authBloc, required this.router});

  final AuthBloc authBloc;
  final dynamic router;

  @override
  Widget build(BuildContext context) {
    return MultiBlocProvider(
      providers: [BlocProvider<AuthBloc>.value(value: authBloc)],
      child: MaterialApp.router(
        title: 'Dzeroth',
        theme: AppTheme.lightTheme(),
        darkTheme: AppTheme.darkTheme(),
        themeMode: ThemeMode.dark,
        routerConfig: router,
        debugShowCheckedModeBanner: false,
      ),
    );
  }
}
