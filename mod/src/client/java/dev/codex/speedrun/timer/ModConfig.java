package dev.codex.speedrun.timer;

import net.fabricmc.loader.api.FabricLoader;

import java.io.IOException;
import java.io.InputStream;
import java.io.OutputStream;
import java.nio.file.Files;
import java.nio.file.Path;
import java.util.Collections;
import java.util.LinkedHashSet;
import java.util.Properties;
import java.util.Set;

final class ModConfig {
    private static final String FILE_NAME = "minecraft-speedrun-timer.properties";
    static final String OFFICIAL_BASE_URL = ModBuildConfig.DEFAULT_BASE_URL;
    static final String MIRROR_BASE_URL = "";

    private String baseUrl = OFFICIAL_BASE_URL;
    private String modToken = "";
    private int hudX = 10;
    private int hudY = 10;
    private int hudReferenceWidth;
    private int hudReferenceHeight;
    private boolean coordinatesEnabled;
    private int coordinatesOffsetY = 59;
    private boolean allowCreative;
    private ActiveRunSession activeRunSession = ActiveRunSession.empty();
    private final Set<String> usedWorlds = new LinkedHashSet<>();

    static ModConfig load() {
        ModConfig config = new ModConfig();
        Path path = configPath();
        if (!Files.exists(path)) {
            return config;
        }

        Properties properties = new Properties();
        try (java.io.BufferedReader reader = Files.newBufferedReader(path, java.nio.charset.StandardCharsets.UTF_8)) {
            properties.load(reader);
            config.baseUrl = properties.getProperty("base_url", OFFICIAL_BASE_URL).trim();
            config.modToken = properties.getProperty("mod_token", "").trim();
            config.hudX = parseInt(properties.getProperty("hud_x"), config.hudX);
            config.hudY = parseInt(properties.getProperty("hud_y"), config.hudY);
            config.hudReferenceWidth = parseInt(properties.getProperty("hud_reference_width"), config.hudReferenceWidth);
            config.hudReferenceHeight = parseInt(properties.getProperty("hud_reference_height"), config.hudReferenceHeight);
            config.coordinatesEnabled = Boolean.parseBoolean(properties.getProperty("show_coordinates", "false"));
            config.coordinatesOffsetY = parseInt(properties.getProperty("coordinates_offset_y"), 59);
            config.allowCreative = Boolean.parseBoolean(properties.getProperty("allow_creative", "false"));
            config.activeRunSession = new ActiveRunSession(
                properties.getProperty("run_session_id", "").trim(),
                properties.getProperty("run_session_world_fingerprint", "").trim(),
                properties.getProperty("run_session_seed_hash", "").trim(),
                parseLong(properties.getProperty("run_session_started_at_ms"), 0L),
                parseLong(properties.getProperty("run_session_expires_at_ms"), 0L),
                properties.getProperty("run_session_phase", "").trim(),
                properties.getProperty("run_session_match_id", "").trim(),
                parseLong(properties.getProperty("run_session_nether_split_ms"), -1L),
                parseLong(properties.getProperty("run_session_end_split_ms"), -1L),
                parseLong(properties.getProperty("run_session_last_seen_ms"), 0L),
                parseLong(properties.getProperty("run_session_gameplay_elapsed_ms"), 0L)
            );
            int usedWorldCount = parseInt(properties.getProperty("used_world_count"), 0);
            for (int index = 0; index < usedWorldCount; index++) {
                String worldKey = properties.getProperty("used_world_" + index, "").trim();
                if (!worldKey.isBlank()) {
                    config.usedWorlds.add(worldKey);
                }
            }
        } catch (IOException ignored) {
        }
        return config;
    }

    void save() throws IOException {
        Properties properties = new Properties();
        properties.setProperty("base_url", baseUrl.trim());
        properties.setProperty("mod_token", modToken.trim());
        properties.setProperty("hud_x", Integer.toString(hudX));
        properties.setProperty("hud_y", Integer.toString(hudY));
        properties.setProperty("hud_reference_width", Integer.toString(hudReferenceWidth));
        properties.setProperty("hud_reference_height", Integer.toString(hudReferenceHeight));
        properties.setProperty("show_coordinates", Boolean.toString(coordinatesEnabled));
        properties.setProperty("coordinates_offset_y", Integer.toString(coordinatesOffsetY));
        properties.setProperty("allow_creative", Boolean.toString(allowCreative));
        if (activeRunSession != null && activeRunSession.isPresent()) {
            properties.setProperty("run_session_id", activeRunSession.id());
            properties.setProperty("run_session_world_fingerprint", activeRunSession.worldFingerprint());
            properties.setProperty("run_session_seed_hash", activeRunSession.seedHash());
            properties.setProperty("run_session_started_at_ms", Long.toString(activeRunSession.startedAtMs()));
            properties.setProperty("run_session_expires_at_ms", Long.toString(activeRunSession.expiresAtMs()));
            properties.setProperty("run_session_phase", activeRunSession.phase());
            properties.setProperty("run_session_match_id", activeRunSession.matchId());
            properties.setProperty("run_session_nether_split_ms", Long.toString(activeRunSession.netherSplitMs()));
            properties.setProperty("run_session_end_split_ms", Long.toString(activeRunSession.endSplitMs()));
            properties.setProperty("run_session_last_seen_ms", Long.toString(activeRunSession.lastSeenMs()));
            properties.setProperty("run_session_gameplay_elapsed_ms", Long.toString(activeRunSession.gameplayElapsedMs()));
        }
        properties.setProperty("used_world_count", Integer.toString(usedWorlds.size()));
        int index = 0;
        for (String worldKey : usedWorlds) {
            properties.setProperty("used_world_" + index, worldKey);
            index++;
        }

        Path path = configPath();
        Files.createDirectories(path.getParent());
        try (java.io.BufferedWriter writer = Files.newBufferedWriter(path, java.nio.charset.StandardCharsets.UTF_8)) {
            properties.store(writer, "Minecraft Speedrun Timer");
        }
    }

    String getBaseUrl() {
        return baseUrl;
    }

    void setBaseUrl(String baseUrl) {
        this.baseUrl = baseUrl == null ? OFFICIAL_BASE_URL : baseUrl.trim();
    }

    String getModToken() {
        return modToken.trim();
    }

    void setModToken(String modToken) {
        this.modToken = modToken == null ? "" : modToken.trim();
    }

    boolean isConfigured() {
        return !getModToken().isBlank();
    }

    boolean isAllowCreative() {
        return allowCreative;
    }

    void setAllowCreative(boolean allowCreative) {
        this.allowCreative = allowCreative;
    }

    int getHudX() {
        return hudX;
    }

    void setHudX(int hudX) {
        this.hudX = hudX;
    }

    int getHudY() {
        return hudY;
    }

    void setHudY(int hudY) {
        this.hudY = hudY;
    }

    int getHudReferenceWidth() {
        return hudReferenceWidth;
    }

    int getHudReferenceHeight() {
        return hudReferenceHeight;
    }

    void setHudReferenceSize(int width, int height) {
        this.hudReferenceWidth = Math.max(1, width);
        this.hudReferenceHeight = Math.max(1, height);
    }

    boolean isCoordinatesEnabled() {
        return coordinatesEnabled;
    }

    void setCoordinatesEnabled(boolean coordinatesEnabled) {
        this.coordinatesEnabled = coordinatesEnabled;
    }

    int getCoordinatesOffsetY() {
        return coordinatesOffsetY;
    }

    void setCoordinatesOffsetY(int coordinatesOffsetY) {
        this.coordinatesOffsetY = coordinatesOffsetY;
    }

    boolean isWorldUsed(String worldKey) {
        return usedWorlds.contains(normalizeWorldKey(worldKey));
    }

    void markWorldUsed(String worldKey) {
        String normalized = normalizeWorldKey(worldKey);
        if (!normalized.isBlank()) {
            usedWorlds.add(normalized);
        }
    }

    Set<String> getUsedWorlds() {
        return Collections.unmodifiableSet(usedWorlds);
    }

    ActiveRunSession getActiveRunSession() {
        return activeRunSession == null ? ActiveRunSession.empty() : activeRunSession;
    }

    void setActiveRunSession(ActiveRunSession activeRunSession) {
        this.activeRunSession = activeRunSession == null ? ActiveRunSession.empty() : activeRunSession;
    }

    void clearActiveRunSession() {
        this.activeRunSession = ActiveRunSession.empty();
    }

    private static Path configPath() {
        return FabricLoader.getInstance().getConfigDir().resolve(FILE_NAME);
    }

    private static String normalizeWorldKey(String value) {
        return value == null ? "" : value.trim();
    }

    private static int parseInt(String value, int fallback) {
        if (value == null || value.isBlank()) {
            return fallback;
        }
        try {
            return Integer.parseInt(value.trim());
        } catch (NumberFormatException exception) {
            return fallback;
        }
    }

    private static long parseLong(String value, long fallback) {
        if (value == null || value.isBlank()) {
            return fallback;
        }
        try {
            return Long.parseLong(value.trim());
        } catch (NumberFormatException exception) {
            return fallback;
        }
    }

    record ActiveRunSession(
        String id,
        String worldFingerprint,
        String seedHash,
        long startedAtMs,
        long expiresAtMs,
        String phase,
        String matchId,
        long netherSplitMs,
        long endSplitMs,
        long lastSeenMs,
        long gameplayElapsedMs
    ) {
        static ActiveRunSession empty() {
            return new ActiveRunSession("", "", "", 0L, 0L, "", "", -1L, -1L, 0L, 0L);
        }

        boolean isPresent() {
            return id != null && !id.isBlank();
        }

        boolean matchesWorld(String worldFingerprint, String seedHash) {
            return isPresent()
                && this.worldFingerprint.equals(worldFingerprint == null ? "" : worldFingerprint)
                && this.seedHash.equals(seedHash == null ? "" : seedHash);
        }

        boolean isExpired(long nowMs) {
            return expiresAtMs > 0L && nowMs >= expiresAtMs;
        }
    }
}
