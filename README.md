# Dimension Science Tournament Kit

White-label toolkit for running a Minecraft speedrun tournament with a public site, Twitch login, streamer cabinet, leaderboard, tournament bracket, admin controls, certificates, achievements, and a Fabric timer mod.

This repository is meant to be copied or forked for your own event. Replace the values in `.env`, add your assets, initialize the database, and publish the result under your own tournament name.

## What Is Included

- Public tournament site with leaderboard, bracket, news, FAQ, guide, and streamer materials
- Twitch OAuth login and local dev/mock login
- Streamer cabinet with profile, avatar, mod token, achievements, and certificate download
- PostgreSQL store for participants, runs, tournaments, matches, applications, sessions, and news
- Admin endpoints and pages for applications, whitelist, tournament control, runs, test mode, and notifications
- Telegram news/support integration and Discord role/channel helpers
- HMAC-signed mod API with nonce replay protection
- Fabric mod for Minecraft Java Edition `1.21.11`
- Default achievement badges that you can keep, replace, or disable

## Quick Start

1. Copy `.env.example` to `.env`.
2. Fill at least these values:

```env
SESSION_SECRET=replace_with_a_real_long_random_secret
MOD_API_KEY=replace_with_a_real_long_random_api_key
MOD_SIGNING_SECRET=replace_with_a_real_long_random_signing_secret
MOD_TOKEN_ENCRYPTION_SECRET=replace_with_a_real_long_random_token_secret
TOURNAMENT_TITLE=Your Tournament Name
SITE_SHORT_NAME=YTN
ORGANIZER_NAME=Your Team
APP_BASE_URL=http://localhost:3000
DATABASE_URL=postgres://postgres:postgres@localhost:5432/tournament?sslmode=disable
```

3. Start PostgreSQL.
4. Initialize the schema:

```bash
go run ./cmd/dbinit
```

5. Optional: add demo data:

```bash
go run ./cmd/seed
```

6. Start the site:

```bash
go run ./cmd/server
```

The service exposes `/health` and `/api/health`.

On Windows you can also run:

```bat
run-local.bat
```

## White-Label Settings

The main branding knobs live in `.env`:

```env
TOURNAMENT_TITLE=Example Speedrun Tournament
SITE_SHORT_NAME=EST
ORGANIZER_NAME=Dimension Science
ORGANIZER_URL=
SUPPORT_URL=
TELEGRAM_URL=
DISCORD_URL=
TWITCH_URL=
GAME_NAME=Minecraft Java Edition
GAME_VERSION=1.21.11
PRIZE_TEXT=
```

These values are exposed to templates as `.Profile` and through `GET /api/public/config`. Use them when you customize pages instead of hardcoding a tournament name in many files.

## Certificates

Certificate backgrounds are PNG files loaded from:

```env
CERTIFICATE_TEMPLATE_DIR=assets/certificates
CERTIFICATE_PREFIX=CERT-2026
```

Create the directory and put these files inside:

```text
assets/certificates/member.png
assets/certificates/playoff.png
assets/certificates/finalist.png
assets/certificates/winner.png
```

Recommended size: use one consistent landscape PNG size for all four templates. The current renderer was built around large print-style backgrounds and overlays:

- avatar around the left/middle area
- display name and Minecraft nickname on the right
- best time and achievement badges near the lower middle
- issue date and certificate number near the bottom

If your template design uses different text positions, edit `internal/web/certificate.go`. Keep the filenames above so other organizers know exactly what to replace.

Local test flow:

1. Add the four PNG files.
2. Set `CERTIFICATE_TEMPLATE_DIR=assets/certificates`.
3. Run the server.
4. Log in as an admin and open `/api/admin/test-certificate?status=member`, `/api/admin/test-certificate?status=playoff`, `/api/admin/test-certificate?status=finalist`, or `/api/admin/test-certificate?status=winner`.
5. Verify that generated text does not collide with your artwork.

Downloaded certificate numbers use `CERTIFICATE_PREFIX`, for example `CERT-2026-123456`.

## Achievements And Badges

Achievements stay enabled by default:

```env
ENABLE_ACHIEVEMENTS=true
BADGE_ASSET_DIR=internal/web/static
```

Default PNG badges live under:

```text
internal/web/static/achievements/
```

You can keep these defaults if you do not have your own art yet. To replace them, keep the same filenames/slugs so existing achievement logic continues to work. To hide the feature for a tournament, set:

```env
ENABLE_ACHIEVEMENTS=false
```

## Repository Layout

- `cmd/server` - main web server entrypoint
- `cmd/dbinit` - database schema initialization/update
- `cmd/seed` - demo data for local development
- `cmd/news-sync` - Telegram public news history sync
- `cmd/whitelist` - local whitelist helper
- `internal/config` - environment configuration and public profile
- `internal/store` - PostgreSQL data access and schema management
- `internal/session` - signed cookie session handling
- `internal/twitch` - Twitch OAuth and Helix API client
- `internal/web` - HTTP handlers, templates, static assets, certificates
- `internal/telegrambot` - Telegram bot, support, news sync
- `internal/discordbot` - Discord notifications and role/channel helpers
- `internal/modrinth` - optional Modrinth release polling
- `mod` - Fabric timer mod
- `assets/certificates` - your certificate PNG templates
- `data/uploads` - runtime uploads; do not commit user uploads

## Docker

```bash
docker compose up --build
```

The Compose stack starts PostgreSQL and the Go application. Uploads are mounted from `data/uploads`, and certificate templates are mounted from `assets/certificates`.

## Production Checklist

- Set `APP_ENV=production`
- Set strong real values for all secrets
- Set `ALLOW_DEV_MOCK_AUTH=false`
- Configure Twitch OAuth credentials and redirect URL
- Configure Telegram/Discord only if you use those integrations
- Put the app behind HTTPS
- Set `AUTH_COOKIE_SECURE=true`
- Set `APP_BASE_URL` and `ALLOWED_ORIGINS` to your real domain
- Add your four certificate templates
- Review all public pages under `internal/web/templates`
- Build and publish your mod with your own site URL and secrets

## Minecraft Mod

The Fabric mod lives in `mod`.

Build it with:

```bash
cd mod
./gradlew build
```

The output jar is created in `mod/build/libs/`.

For a real public release, update the mod metadata in:

```text
mod/gradle.properties
mod/src/main/resources/fabric.mod.json
mod/src/main/resources/assets/minecraft_speedrun_timer/lang/en_us.json
mod/src/main/resources/assets/minecraft_speedrun_timer/lang/ru_ru.json
```

Then build with real `MOD_API_KEY` and `MOD_SIGNING_SECRET` values that match your server.
