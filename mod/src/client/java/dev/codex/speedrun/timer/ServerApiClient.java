package dev.codex.speedrun.timer;

import com.google.gson.JsonArray;
import com.google.gson.JsonElement;
import com.google.gson.JsonObject;
import com.google.gson.JsonParseException;
import com.google.gson.JsonParser;

import javax.crypto.Mac;
import javax.crypto.spec.SecretKeySpec;
import java.io.IOException;
import java.net.URI;
import java.net.http.HttpClient;
import java.net.http.HttpRequest;
import java.net.http.HttpResponse;
import java.nio.charset.StandardCharsets;
import java.security.GeneralSecurityException;
import java.security.SecureRandom;
import java.time.Duration;
import java.time.Instant;
import java.time.format.DateTimeFormatter;
import java.util.ArrayList;
import java.util.HexFormat;
import java.util.LinkedHashMap;
import java.util.List;
import java.util.Map;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.function.Consumer;

final class ServerApiClient {
    private static final SecureRandom RANDOM = new SecureRandom();

    private final HttpClient httpClient = HttpClient.newBuilder()
        .connectTimeout(Duration.ofSeconds(4))
        .followRedirects(HttpClient.Redirect.NORMAL)
        .build();
    private final ExecutorService executor = Executors.newSingleThreadExecutor(runnable -> {
        Thread thread = new Thread(runnable, "minecraft-speedrun-api");
        thread.setDaemon(true);
        return thread;
    });
    private String activeBaseUrl = null;

    void authorizeAsync(ModConfig config, Consumer<ApiResult> callback) {
        executor.execute(() -> callback.accept(authorize(config)));
    }

    void submitRunAsync(ModConfig config, RunTimerState.Snapshot snapshot, Consumer<ApiResult> callback) {
        executor.execute(() -> callback.accept(submitRun(config, snapshot)));
    }

    void createRunSessionAsync(ModConfig config, String worldFingerprint, String seedHash, Consumer<ApiResult> callback) {
        executor.execute(() -> callback.accept(createRunSession(config, worldFingerprint, seedHash)));
    }

    void validateRunSessionAsync(ModConfig config, String runSessionId, String worldFingerprint, String seedHash, Consumer<ApiResult> callback) {
        executor.execute(() -> callback.accept(validateRunSession(config, runSessionId, worldFingerprint, seedHash)));
    }

    void markMatchReadyAsync(ModConfig config, String matchId, Consumer<ApiResult> callback) {
        executor.execute(() -> callback.accept(markMatchReady(config, matchId)));
    }

    private ApiResult authorize(ModConfig config) {
        if (!config.isConfigured()) {
            return ApiResult.error(MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.set_token"));
        }

        try {
            String payload = canonicalJson(Map.of("modToken", normalizeModToken(config.getModToken())));
            HttpResponse<String> response = sendRequestWithFallback(config, "/api/mod/authorize", payload, Duration.ofSeconds(15));
            String body = response.body();
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                JsonObject object = parseObject(body);
                JsonObject participant = getObject(object, "participant");
                JsonObject tournament = getObject(object, "tournament");
                String twitchLogin = getString(participant, "twitchLogin", getString(object, "twitchLogin", ""));
                if (twitchLogin.isBlank()) {
                    twitchLogin = getString(participant, "twitch_login", getString(object, "twitch_login", ""));
                }
                String minecraftNick = getString(participant, "minecraftNick", getString(object, "minecraftNick", ""));
                if (minecraftNick.isBlank()) {
                    minecraftNick = getString(participant, "minecraft_nick", getString(object, "minecraft_nick", ""));
                }
                String displayLabel = twitchLogin;
                if (displayLabel.isBlank()) {
                    displayLabel = getString(participant, "twitchUserId", getString(object, "twitchUserId", "authorized"));
                }
                if (!displayLabel.equals("authorized") && !minecraftNick.isBlank()) {
                    displayLabel = displayLabel + " (" + minecraftNick + ")";
                }

                String phase = getString(tournament, "phase", getString(object, "phase", ""));
                String phaseLabel = getString(tournament, "phaseLabel", getString(object, "phaseLabel", phase));
                String matchId = getString(tournament, "matchId", getString(object, "matchId", ""));
                String worldSeed = getString(tournament, "worldSeed", getString(object, "worldSeed", ""));
                boolean canStartOfficialRun = getBoolean(tournament, "canStartOfficialRun", getBoolean(object, "canStartOfficialRun", true));
                String rawTournamentReason = getString(tournament, "reason", getString(object, "reason", ""));
                String tournamentReason = rawTournamentReason.isBlank() ? "" : friendlyReason(rawTournamentReason);
                MinecraftSpeedrunModClient.LOGGER.info("[API] Authorize successful: login={}, phase={}, canStartOfficialRun={}", displayLabel, phase, canStartOfficialRun);
                return ApiResult.success(
                    displayLabel,
                    body,
                    List.of(),
                    phase,
                    phaseLabel,
                    canStartOfficialRun,
                    tournamentReason,
                    matchId,
                    worldSeed
                );
            }
            String errorDetail = apiErrorDetail(response);
            MinecraftSpeedrunModClient.LOGGER.error("[API] Authorize failed: status={}, body={}", response.statusCode(), body);
            return ApiResult.error(errorDetail);
        } catch (InterruptedException exception) {
            Thread.currentThread().interrupt();
            MinecraftSpeedrunModClient.LOGGER.error("[API] Authorize interrupted", exception);
            return ApiResult.error("[Exception] Interrupted: " + exception.getMessage());
        } catch (IOException | RuntimeException exception) {
            MinecraftSpeedrunModClient.LOGGER.error("[API] Authorize exception", exception);
            return ApiResult.error("[Exception] " + exception.getClass().getSimpleName() + ": " + exception.getMessage());
        }
    }

    private ApiResult submitRun(ModConfig config, RunTimerState.Snapshot snapshot) {
        if (!config.isConfigured()) {
            return ApiResult.error(MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.token_missing"));
        }

        try {
            Map<String, Object> payload = new LinkedHashMap<>();
            if (snapshot.endSplitMs() != null) {
                payload.put("endSplitMs", snapshot.endSplitMs());
            }
            payload.put("finishedAt", DateTimeFormatter.ISO_INSTANT.format(snapshot.finishedAtUtc()));
            if (!snapshot.matchId().isBlank()) {
                payload.put("matchId", snapshot.matchId());
            }
            payload.put("modToken", normalizeModToken(config.getModToken()));
            if (snapshot.netherSplitMs() != null) {
                payload.put("netherSplitMs", snapshot.netherSplitMs());
            }
            if (!snapshot.phase().isBlank()) {
                payload.put("phase", snapshot.phase());
            }
            if (!snapshot.runSessionId().isBlank()) {
                payload.put("runSessionId", snapshot.runSessionId());
            }
            if (!snapshot.seedHash().isBlank()) {
                payload.put("seedHash", snapshot.seedHash());
            }
            payload.put("startedAt", DateTimeFormatter.ISO_INSTANT.format(snapshot.startedAtUtc()));
            payload.put("timeMs", snapshot.gameplayElapsedMs());
            if (!snapshot.worldFingerprint().isBlank()) {
                payload.put("worldFingerprint", snapshot.worldFingerprint());
            }

            String payloadJson = canonicalJson(payload);
            HttpResponse<String> response = sendRequestWithFallback(config, "/api/mod/runs", payloadJson, Duration.ofSeconds(20));
            String body = response.body();
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                MinecraftSpeedrunModClient.LOGGER.info("[API] Run submitted successfully: body={}", body);
                return ApiResult.success("", body, extractJustUnlockedAchievements(body));
            }
            String errorDetail = apiErrorDetail(response);
            MinecraftSpeedrunModClient.LOGGER.error("[API] Run submission failed: status={}, body={}", response.statusCode(), body);
            return ApiResult.error(errorDetail);
        } catch (InterruptedException exception) {
            Thread.currentThread().interrupt();
            MinecraftSpeedrunModClient.LOGGER.error("[API] Run submission interrupted", exception);
            return ApiResult.error("[Exception] Interrupted: " + exception.getMessage());
        } catch (IOException | RuntimeException exception) {
            MinecraftSpeedrunModClient.LOGGER.error("[API] Run submission exception", exception);
            return ApiResult.error("[Exception] " + exception.getClass().getSimpleName() + ": " + exception.getMessage());
        }
    }

    private ApiResult createRunSession(ModConfig config, String worldFingerprint, String seedHash) {
        return runSessionRequest(config, "/api/mod/run-sessions", "", worldFingerprint, seedHash);
    }

    private ApiResult validateRunSession(ModConfig config, String runSessionId, String worldFingerprint, String seedHash) {
        return runSessionRequest(config, "/api/mod/run-sessions/validate", runSessionId, worldFingerprint, seedHash);
    }

    private ApiResult runSessionRequest(ModConfig config, String path, String runSessionId, String worldFingerprint, String seedHash) {
        if (!config.isConfigured()) {
            return ApiResult.error(MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.token_missing"));
        }

        try {
            Map<String, Object> payload = new LinkedHashMap<>();
            payload.put("modToken", normalizeModToken(config.getModToken()));
            if (runSessionId != null && !runSessionId.isBlank()) {
                payload.put("runSessionId", runSessionId);
            }
            payload.put("seedHash", seedHash == null ? "" : seedHash);
            payload.put("worldFingerprint", worldFingerprint == null ? "" : worldFingerprint);

            String payloadJson = canonicalJson(payload);
            HttpResponse<String> response = sendRequestWithFallback(config, path, payloadJson, Duration.ofSeconds(15));
            String body = response.body();
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                JsonObject object = parseObject(body);
                JsonObject participant = getObject(object, "participant");
                JsonObject tournament = getObject(object, "tournament");
                String twitchLogin = getString(participant, "twitchLogin", getString(object, "twitchLogin", ""));
                String phase = getString(tournament, "phase", getString(object, "phase", ""));
                String phaseLabel = getString(tournament, "phaseLabel", getString(object, "phaseLabel", phase));
                String matchId = getString(tournament, "matchId", getString(object, "matchId", ""));
                String worldSeed = getString(tournament, "worldSeed", getString(object, "worldSeed", ""));
                String sessionId = getString(object, "runSessionId", "");
                String expiresAt = getString(object, "expiresAt", "");
                MinecraftSpeedrunModClient.LOGGER.info("[API] Run session ok: path={}, sessionId={}, phase={}", path, sessionId, phase);
                return ApiResult.success(
                    twitchLogin,
                    body,
                    List.of(),
                    phase,
                    phaseLabel,
                    true,
                    "",
                    matchId,
                    worldSeed,
                    sessionId,
                    expiresAt
                );
            }
            String errorDetail = apiErrorDetail(response);
            String rawReason = errorReason(body, "");
            MinecraftSpeedrunModClient.LOGGER.error("[API] Run session failed: path={}, status={}, body={}", path, response.statusCode(), body);
            return ApiResult.error(errorDetail, rawReason);
        } catch (InterruptedException exception) {
            Thread.currentThread().interrupt();
            MinecraftSpeedrunModClient.LOGGER.error("[API] Run session interrupted: path={}", path, exception);
            return ApiResult.error("[Exception] Interrupted: " + exception.getMessage());
        } catch (IOException | RuntimeException exception) {
            MinecraftSpeedrunModClient.LOGGER.error("[API] Run session exception: path={}", path, exception);
            return ApiResult.error("[Exception] " + exception.getClass().getSimpleName() + ": " + exception.getMessage());
        }
    }

    private ApiResult markMatchReady(ModConfig config, String matchId) {
        if (!config.isConfigured()) {
            return ApiResult.error(MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.token_missing"));
        }

        try {
            Map<String, Object> payload = new LinkedHashMap<>();
            if (matchId != null && !matchId.isBlank()) {
                payload.put("matchId", matchId);
            }
            payload.put("modToken", normalizeModToken(config.getModToken()));

            String payloadJson = canonicalJson(payload);
            HttpResponse<String> response = sendRequestWithFallback(config, "/api/mod/matches/ready", payloadJson, Duration.ofSeconds(15));
            String body = response.body();
            if (response.statusCode() >= 200 && response.statusCode() < 300) {
                JsonObject object = parseObject(body);
                String phase = getString(object, "phase", "");
                String phaseLabel = getString(object, "phaseLabel", phase);
                String responseMatchId = getString(object, "matchId", matchId == null ? "" : matchId);
                String worldSeed = getString(object, "worldSeed", "");
                boolean canStartOfficialRun = getBoolean(object, "canStartOfficialRun", false);
                String rawTournamentReason = getString(object, "reason", "");
                String tournamentReason = rawTournamentReason.isBlank() ? "" : friendlyReason(rawTournamentReason);
                MinecraftSpeedrunModClient.LOGGER.info("[API] Match ready check successful: phase={}, canStartOfficialRun={}", phase, canStartOfficialRun);
                return ApiResult.success(
                    "",
                    body,
                    List.of(),
                    phase,
                    phaseLabel,
                    canStartOfficialRun,
                    tournamentReason,
                    responseMatchId,
                    worldSeed
                );
            }
            String errorDetail = apiErrorDetail(response);
            MinecraftSpeedrunModClient.LOGGER.error("[API] Match ready check failed: status={}, body={}", response.statusCode(), body);
            return ApiResult.error(errorDetail);
        } catch (InterruptedException exception) {
            Thread.currentThread().interrupt();
            MinecraftSpeedrunModClient.LOGGER.error("[API] Match ready check interrupted", exception);
            return ApiResult.error("[Exception] Interrupted: " + exception.getMessage());
        } catch (IOException | RuntimeException exception) {
            MinecraftSpeedrunModClient.LOGGER.error("[API] Match ready check exception", exception);
            return ApiResult.error("[Exception] " + exception.getClass().getSimpleName() + ": " + exception.getMessage());
        }
    }

    private HttpResponse<String> sendRequestWithFallback(ModConfig config, String path, String payload, Duration timeout) throws IOException, InterruptedException {
        String base = activeBaseUrl != null ? activeBaseUrl : config.getBaseUrl();
        HttpRequest request = HttpRequest.newBuilder(URI.create(base + path))
            .timeout(timeout)
            .header("Content-Type", "application/json")
            .POST(HttpRequest.BodyPublishers.ofString(payload))
            .build();

        try {
            HttpResponse<String> response = httpClient.send(request, HttpResponse.BodyHandlers.ofString());
            if (response.statusCode() == 302 && base.equals(ModConfig.OFFICIAL_BASE_URL)) {
                MinecraftSpeedrunModClient.LOGGER.warn("[API] Official URL returned 302. Falling back to mirror.");
                activeBaseUrl = ModConfig.MIRROR_BASE_URL;
                config.setBaseUrl(ModConfig.MIRROR_BASE_URL);
                try {
                    config.save();
                } catch (IOException ignored) {}
                return sendRequestWithFallback(config, path, payload, timeout);
            }
            if (base.equals(ModConfig.OFFICIAL_BASE_URL)) {
                String contentType = response.headers().firstValue("Content-Type").orElse("").toLowerCase();
                String body = response.body();
                if (contentType.contains("text/html") || (body != null && body.trim().startsWith("<"))) {
                    MinecraftSpeedrunModClient.LOGGER.warn("[API] HTML response received from official URL (possibly intercepted). Falling back to mirror.");
                    activeBaseUrl = ModConfig.MIRROR_BASE_URL;
                    config.setBaseUrl(ModConfig.MIRROR_BASE_URL);
                    try {
                        config.save();
                    } catch (IOException ignored) {}
                    return sendRequestWithFallback(config, path, payload, timeout);
                }
            }
            return response;
        } catch (IOException exception) {
            if (base.equals(ModConfig.OFFICIAL_BASE_URL)) {
                MinecraftSpeedrunModClient.LOGGER.warn("[API] Connection to official URL failed. Falling back to mirror. Error: {}", exception.getMessage());
                activeBaseUrl = ModConfig.MIRROR_BASE_URL;
                config.setBaseUrl(ModConfig.MIRROR_BASE_URL);
                try {
                    config.save();
                } catch (IOException ignored) {}
                return sendRequestWithFallback(config, path, payload, timeout);
            }
            throw exception;
        }
    }


    private static String normalizeModToken(String token) {
        String trimmed = token == null ? "" : token.trim();
        if (trimmed.isBlank()) {
            return "";
        }
        if (trimmed.startsWith("msrmod_")) {
            return trimmed;
        }
        if (trimmed.matches("[0-9a-fA-F]{40}") || trimmed.matches("[0-9a-fA-F]{48}")) {
            return "msrmod_" + trimmed.toLowerCase();
        }
        return trimmed;
    }

    private static String friendlyReason(String reason) {
        return switch (reason) {
            case "mod_token_expired" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.token_expired");
            case "invalid_mod_token", "not_whitelisted" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.token_not_accepted");
            case "tournament_not_running" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.tournament_not_running");
            case "stream_offline" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.stream_offline");
            case "stream_not_minecraft" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.stream_not_minecraft");
            case "stream_check_unavailable" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.stream_check_unavailable");
            case "match_window_inactive" -> "Матч сейчас не активен";
            case "match_not_started" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.match_not_started");
            case "test_runs_disabled" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.test_route_disabled");
            case "run_session_missing" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.run_session_missing");
            case "run_session_closed" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.run_session_closed");
            case "run_session_expired" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.run_session_expired");
            case "run_session_world_mismatch" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.run_session_world_mismatch");
            case "run_session_context_changed" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.run_session_context_changed");
            case "run_session_world_required" -> MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.run_session_world_required");
            case "match_lost" -> "Вы второй! Соперник уже финишировал.";
            default -> reason == null || reason.isBlank() ? MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.authorization_failed") : reason;
        };
    }

    private static String canonicalJson(Map<String, ?> values) {
        StringBuilder builder = new StringBuilder();
        builder.append('{');
        boolean first = true;
        for (Map.Entry<String, ?> entry : values.entrySet().stream().sorted(Map.Entry.comparingByKey()).toList()) {
            if (!first) {
                builder.append(',');
            }
            builder.append('"').append(escape(entry.getKey())).append('"').append(':');
            Object value = entry.getValue();
            if (value == null) {
                builder.append("null");
            } else if (value instanceof Number || value instanceof Boolean) {
                builder.append(value);
            } else {
                builder.append('"').append(escape(value.toString())).append('"');
            }
            first = false;
        }

        builder.append('}');
        return builder.toString();
    }

    private static String escape(String value) {
        return value
            .replace("\\", "\\\\")
            .replace("\"", "\\\"")
            .replace("\n", "\\n")
            .replace("\r", "\\r")
            .replace("\t", "\\t");
    }

    private static List<AchievementUnlock> extractJustUnlockedAchievements(String body) {
        List<AchievementUnlock> achievements = new ArrayList<>();
        JsonArray items = getArray(parseObject(body), "achievements");
        if (items == null) {
            return achievements;
        }
        for (JsonElement item : items) {
            if (!item.isJsonObject()) {
                continue;
            }
            JsonObject object = item.getAsJsonObject();
            if (!getBoolean(object, "justUnlocked", false)) {
                continue;
            }
            String slug = getString(object, "slug", "");
            String name = getString(object, "name", "");
            if (!name.isBlank()) {
                achievements.add(new AchievementUnlock(slug, name));
            }
        }
        return achievements;
    }

    private static String errorReason(String body, String fallback) {
        JsonObject object = parseObject(body);
        String reason = getString(object, "reason", "");
        if (!reason.isBlank()) {
            return reason;
        }
        return getString(object, "error", fallback);
    }

    private static String apiErrorDetail(HttpResponse<String> response) {
        int statusCode = response.statusCode();
        String body = response.body();
        if (statusCode >= 300 && statusCode < 400) {
            String location = response.headers().firstValue("Location").orElse("unknown");
            return String.format("[HTTP %d] Redirected to: %s", statusCode, location);
        }
        String serverMsg = errorReason(body, "");
        if (!serverMsg.isBlank()) {
            return String.format("[HTTP %d] %s", statusCode, friendlyReason(serverMsg));
        }
        String cleanedBody = body == null ? "" : body.trim().replaceAll("\\s+", " ");
        if (cleanedBody.length() > 80) {
            cleanedBody = cleanedBody.substring(0, 77) + "...";
        }
        return String.format("[HTTP %d] %s", statusCode, cleanedBody.isBlank() ? "Response empty" : cleanedBody);
    }

    private static JsonObject parseObject(String body) {
        if (body == null || body.isBlank()) {
            return new JsonObject();
        }
        JsonElement parsed;
        try {
            parsed = JsonParser.parseString(body);
        } catch (JsonParseException exception) {
            return new JsonObject();
        }
        if (!parsed.isJsonObject()) {
            return new JsonObject();
        }
        return parsed.getAsJsonObject();
    }

    private static String getString(JsonObject object, String name, String fallback) {
        JsonElement value = object.get(name);
        if (value == null || value.isJsonNull()) {
            return fallback;
        }
        try {
            return value.getAsString();
        } catch (UnsupportedOperationException | IllegalStateException exception) {
            return fallback;
        }
    }

    private static boolean getBoolean(JsonObject object, String name, boolean fallback) {
        JsonElement value = object.get(name);
        if (value == null || value.isJsonNull()) {
            return fallback;
        }
        try {
            return value.getAsBoolean();
        } catch (UnsupportedOperationException | IllegalStateException exception) {
            return fallback;
        }
    }

    private static JsonObject getObject(JsonObject object, String name) {
        JsonElement value = object.get(name);
        return value != null && value.isJsonObject() ? value.getAsJsonObject() : new JsonObject();
    }

    private static JsonArray getArray(JsonObject object, String name) {
        JsonElement value = object.get(name);
        return value != null && value.isJsonArray() ? value.getAsJsonArray() : null;
    }

    record AchievementUnlock(String slug, String name) {
    }

    record ApiResult(boolean ok, String message, String body, List<AchievementUnlock> achievements, String phase, String phaseLabel, boolean canStartOfficialRun, String tournamentReason, String matchId, String worldSeed, String runSessionId, String runSessionExpiresAt) {
        static ApiResult success(String message, String body) {
            return success(message, body, List.of());
        }

        static ApiResult success(String message, String body, List<AchievementUnlock> achievements) {
            return success(message, body, achievements, "", "", true, "", "", "");
        }

        static ApiResult success(String message, String body, List<AchievementUnlock> achievements, String phase, String phaseLabel, boolean canStartOfficialRun, String tournamentReason, String matchId, String worldSeed) {
            return success(message, body, achievements, phase, phaseLabel, canStartOfficialRun, tournamentReason, matchId, worldSeed, "", "");
        }

        static ApiResult success(String message, String body, List<AchievementUnlock> achievements, String phase, String phaseLabel, boolean canStartOfficialRun, String tournamentReason, String matchId, String worldSeed, String runSessionId, String runSessionExpiresAt) {
            return new ApiResult(true, message, body, achievements, phase, phaseLabel, canStartOfficialRun, tournamentReason, matchId, worldSeed, runSessionId, runSessionExpiresAt);
        }

        static ApiResult error(String message) {
            return error(message, "");
        }

        static ApiResult error(String message, String tournamentReason) {
            return new ApiResult(false, message, "", List.of(), "", "", false, tournamentReason, "", "", "", "");
        }
    }
}
