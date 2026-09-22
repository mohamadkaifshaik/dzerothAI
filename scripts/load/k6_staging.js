// TARGET: Staging only. Do NOT run against production.
//
// Dzeroth staging load test — Phase 11C performance baseline.
//
// Usage:
//   k6 run \
//     -e BASE_URL=https://staging.example.com \
//     -e API_TOKEN=<valid_staging_access_token> \
//     scripts/load/k6_staging.js
//
// Required environment variables:
//   BASE_URL    — full base URL of the staging API (no trailing slash)
//   API_TOKEN   — a valid JWT access token for an existing staging user
//
// Optional environment variables:
//   POST_VU_COUNT   — VU count for the create-post scenario (default: 10)
//   FEED_VU_COUNT   — VU count for the home-feed scenario (default: 10)
//   AUTH_VU_COUNT   — VU count for the auth/login scenario (default: 5)
//   DURATION        — test duration string (default: "30s")
//
// Required test data:
//   - A staging user whose credentials are embedded in a separate seed script.
//     The API_TOKEN must be pre-minted; this script does not register users.
//   - The staging database must have at least one user with followers so the
//     home feed returns non-empty pages.
//
// Thresholds (staging only — these are NOT production SLOs):
//   - http_req_duration p(95) < 2000ms
//   - http_req_failed < 5%
//
// Notes:
//   - The create-post scenario uses 'original' post type to avoid needing
//     share_initiated_at timing logic in the load test.
//   - The auth scenario uses invalid credentials to test error-path latency;
//     adjust LOGIN_EMAIL/LOGIN_PASSWORD if valid-credential latency is needed.

import http from 'k6/http';
import { check, sleep } from 'k6';
import { Counter, Rate, Trend } from 'k6/metrics';

// ---------------------------------------------------------------------------
// Configuration
// ---------------------------------------------------------------------------

const BASE_URL = __ENV.BASE_URL || 'https://staging.example.com';
const API_TOKEN = __ENV.API_TOKEN || '';
const POST_VU_COUNT = parseInt(__ENV.POST_VU_COUNT || '10', 10);
const FEED_VU_COUNT = parseInt(__ENV.FEED_VU_COUNT || '10', 10);
const AUTH_VU_COUNT = parseInt(__ENV.AUTH_VU_COUNT || '5', 10);
const DURATION = __ENV.DURATION || '30s';

// ---------------------------------------------------------------------------
// Custom metrics
// ---------------------------------------------------------------------------

const postCreationErrors = new Counter('post_creation_errors');
const feedErrors = new Counter('feed_errors');
const authErrors = new Counter('auth_errors');
const feedTerminated = new Counter('feed_terminated');
const feedMetricLeakage = new Counter('feed_metric_leakage_violations');

// ---------------------------------------------------------------------------
// k6 options — 3 scenarios
// ---------------------------------------------------------------------------

export const options = {
  scenarios: {
    // Scenario 1: Auth / login round-trip
    auth_login: {
      executor: 'constant-vus',
      exec: 'authScenario',
      vus: AUTH_VU_COUNT,
      duration: DURATION,
      tags: { scenario: 'auth' },
    },

    // Scenario 2: Create original post (authenticated)
    create_post: {
      executor: 'constant-vus',
      exec: 'createPostScenario',
      vus: POST_VU_COUNT,
      duration: DURATION,
      tags: { scenario: 'post' },
    },

    // Scenario 3: Get home feed (authenticated)
    home_feed: {
      executor: 'constant-vus',
      exec: 'homeFeedScenario',
      vus: FEED_VU_COUNT,
      duration: DURATION,
      tags: { scenario: 'feed' },
    },
  },

  thresholds: {
    // Staging-only thresholds — conservative, not production SLOs.
    'http_req_duration{scenario:post}': ['p(95)<2000'],
    'http_req_duration{scenario:feed}': ['p(95)<2000'],
    'http_req_duration{scenario:auth}': ['p(95)<2000'],
    http_req_failed: ['rate<0.05'],
    feed_metric_leakage_violations: ['count==0'],
  },
};

// ---------------------------------------------------------------------------
// Scenario: auth / login (error-path latency baseline)
// ---------------------------------------------------------------------------

export function authScenario() {
  const url = `${BASE_URL}/api/v1/auth/login`;
  const payload = JSON.stringify({
    email: 'loadtest-nonexistent@staging.invalid',
    password: 'WrongPassword999!',
  });
  const params = {
    headers: { 'Content-Type': 'application/json' },
    tags: { name: 'login_error_path' },
  };

  const res = http.post(url, payload, params);

  const ok = check(res, {
    'auth login returns 4xx (expected for invalid creds)': (r) =>
      r.status >= 400 && r.status < 500,
  });
  if (!ok) {
    authErrors.add(1);
  }

  sleep(1);
}

// ---------------------------------------------------------------------------
// Scenario: create original post
// ---------------------------------------------------------------------------

export function createPostScenario() {
  if (!API_TOKEN) {
    // Skip if no token configured — do not panic the test.
    sleep(1);
    return;
  }

  const url = `${BASE_URL}/api/v1/posts`;
  const payload = JSON.stringify({
    post_type: 'original',
    content: `Load test post at ${Date.now()}`,
  });
  const params = {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${API_TOKEN}`,
    },
    tags: { name: 'create_post' },
  };

  const res = http.post(url, payload, params);

  const ok = check(res, {
    'create post status is 201 or 429': (r) =>
      r.status === 201 || r.status === 429,
    'create post response has no metric fields': (r) => {
      if (r.status !== 201) return true;
      try {
        const body = JSON.parse(r.body);
        const post = body.post || {};
        const forbidden = [
          'like_count',
          'impression_count',
          'bookmark_count',
          'retweet_count',
          'follower_count',
          'view_count',
          'share_count',
        ];
        for (const field of forbidden) {
          if (field in post) {
            feedMetricLeakage.add(1);
            return false;
          }
        }
        return true;
      } catch (_) {
        return true;
      }
    },
  });
  if (!ok) {
    postCreationErrors.add(1);
  }

  sleep(1);
}

// ---------------------------------------------------------------------------
// Scenario: get home feed
// ---------------------------------------------------------------------------

export function homeFeedScenario() {
  if (!API_TOKEN) {
    sleep(1);
    return;
  }

  const url = `${BASE_URL}/api/v1/feeds/home`;
  const params = {
    headers: {
      'Content-Type': 'application/json',
      Authorization: `Bearer ${API_TOKEN}`,
    },
    tags: { name: 'home_feed' },
  };

  const res = http.get(url, params);

  const ok = check(res, {
    'home feed status is 200': (r) => r.status === 200,
    'home feed has items array': (r) => {
      try {
        const body = JSON.parse(r.body);
        return Array.isArray(body.items);
      } catch (_) {
        return false;
      }
    },
    'home feed has no public metric fields': (r) => {
      if (r.status !== 200) return true;
      try {
        const body = JSON.parse(r.body);
        const items = body.items || [];
        const forbidden = [
          'like_count',
          'impression_count',
          'bookmark_count',
          'retweet_count',
          'follower_count',
          'view_count',
          'share_count',
        ];
        for (const item of items) {
          for (const field of forbidden) {
            if (field in item) {
              feedMetricLeakage.add(1);
              return false;
            }
          }
        }
        return true;
      } catch (_) {
        return true;
      }
    },
    'home feed has terminated field': (r) => {
      if (r.status !== 200) return true;
      try {
        const body = JSON.parse(r.body);
        if (body.terminated === true) {
          feedTerminated.add(1);
        }
        return 'terminated' in body;
      } catch (_) {
        return false;
      }
    },
  });
  if (!ok) {
    feedErrors.add(1);
  }

  sleep(1);
}
