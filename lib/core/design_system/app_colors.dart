import 'package:flutter/material.dart';

/// Dzeroth color palette.
///
/// Dzeroth is a text-first platform with a dark-mode default.
/// The primary color is deep amber — distinct from X/Twitter blue.
/// Do not add social-validation-metric colors (likes, impressions, etc.)
/// to the public palette.
abstract final class AppColors {
  // Primary — deep amber
  static const Color primary = Color(0xFFE8A020);
  static const Color primaryDark = Color(0xFFC47E0A);
  static const Color primaryLight = Color(0xFFFFC84A);
  static const Color onPrimary = Color(0xFF1A0F00);

  // Dark backgrounds
  static const Color backgroundDark = Color(0xFF0D0D0D);
  static const Color surfaceDark = Color(0xFF1A1A1A);
  static const Color surfaceVariantDark = Color(0xFF242424);
  static const Color cardDark = Color(0xFF1F1F1F);

  // Light backgrounds
  static const Color backgroundLight = Color(0xFFF8F8F8);
  static const Color surfaceLight = Color(0xFFFFFFFF);
  static const Color surfaceVariantLight = Color(0xFFF0F0F0);
  static const Color cardLight = Color(0xFFFFFFFF);

  // Text — dark theme
  static const Color textPrimaryDark = Color(0xFFEEEEEE);
  static const Color textSecondaryDark = Color(0xFF9E9E9E);
  static const Color textDisabledDark = Color(0xFF616161);

  // Text — light theme
  static const Color textPrimaryLight = Color(0xFF111111);
  static const Color textSecondaryLight = Color(0xFF616161);
  static const Color textDisabledLight = Color(0xFFBDBDBD);

  // Semantic
  static const Color error = Color(0xFFCF4444);
  static const Color errorContainer = Color(0xFF3B0E0E);
  static const Color onError = Color(0xFFFFFFFF);
  static const Color success = Color(0xFF4CAF50);
  static const Color warning = Color(0xFFFFA000);

  // Dividers / borders
  static const Color dividerDark = Color(0xFF2C2C2C);
  static const Color dividerLight = Color(0xFFE0E0E0);

  // Outline / inactive
  static const Color outlineDark = Color(0xFF3C3C3C);
  static const Color outlineLight = Color(0xFFBDBDBD);
}
