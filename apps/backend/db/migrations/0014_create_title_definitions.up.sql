-- 0014_create_title_definitions.up.sql
--
-- Creates the title_definitions lookup table for the Dzeroth Title System
-- (Phase 1, Title System v1).
--
-- ID: Application-generated UUID v7 (ADR 0004). Seed rows use fixed
-- well-known UUIDs in the 0000-7000-8000 namespace to be idempotent and
-- stable across all environments. No DEFAULT gen_random_uuid().
-- Timestamps: TIMESTAMPTZ stored in UTC (IDENTIFIERS_AND_TIME.md).
--
-- category: constrained to ('milestone', 'niche', 'performance').
-- is_revocable: FALSE means the title can never be taken away once earned.
-- is_active: FALSE titles are not visible to users and cannot be newly awarded.
--
-- display_name length is enforced at 1–20 characters by constraint.
-- This matches the maximum length for public title badges in the UI spec.
--
-- Seed data includes the Phase 1 launch titles. The top_1pct_creator row is
-- seeded with is_active = FALSE (coming soon) and must not be awarded by any
-- engine until that flag is set to TRUE via a subsequent migration.

CREATE TABLE title_definitions (
    id           UUID        NOT NULL,
    slug         TEXT        NOT NULL,
    display_name TEXT        NOT NULL,
    description  TEXT,
    category     VARCHAR(20) NOT NULL,
    is_revocable BOOLEAN     NOT NULL DEFAULT FALSE,
    is_active    BOOLEAN     NOT NULL DEFAULT TRUE,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ NOT NULL DEFAULT now(),

    CONSTRAINT title_definitions_pkey
        PRIMARY KEY (id),

    CONSTRAINT title_definitions_slug_key
        UNIQUE (slug),

    CONSTRAINT title_definitions_display_name_length
        CHECK (char_length(display_name) BETWEEN 1 AND 20),

    CONSTRAINT title_definitions_category_values
        CHECK (category IN ('milestone', 'niche', 'performance'))
);

INSERT INTO title_definitions
    (id, slug, display_name, description, category, is_revocable, is_active)
VALUES
    ('00000000-0000-7000-8000-000000000001',
     'founding_member', 'Founding Member',
     'Joined Dzeroth within the first 30 days of platform launch.',
     'milestone', FALSE, TRUE),

    ('00000000-0000-7000-8000-000000000002',
     'centurion', 'Centurion',
     'Published 100 posts on Dzeroth.',
     'milestone', FALSE, TRUE),

    ('00000000-0000-7000-8000-000000000003',
     'trendsetter', 'Trendsetter',
     'Received over 10,000 post likes in a rolling 30-day window.',
     'performance', TRUE, TRUE),

    ('00000000-0000-7000-8000-000000000004',
     'niche_guru_tech', 'Niche Guru: Tech',
     'Over 15 posts using #Tech or #Gadgets with over 5% engagement in 30 days.',
     'niche', TRUE, TRUE),

    ('00000000-0000-7000-8000-000000000005',
     'niche_guru_fitness', 'Niche Guru: Fitness',
     'Over 15 posts using #Fitness or #Workout with over 5% engagement in 30 days.',
     'niche', TRUE, TRUE),

    ('00000000-0000-7000-8000-000000000006',
     'top_1pct_creator', 'Top 1% Creator',
     'Top 1% of all active creators globally by engagement. Coming soon.',
     'performance', TRUE, FALSE);
