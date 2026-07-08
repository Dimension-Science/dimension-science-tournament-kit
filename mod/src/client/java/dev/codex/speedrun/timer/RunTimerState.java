package dev.codex.speedrun.timer;

import java.time.Instant;

final class RunTimerState {
    private Instant startedAtUtc;
    private long wallClockStartMs;
    private long wallClockElapsedMs;
    private long gameplayElapsedMs;
    private long lastTickMs;
    private Long netherSplitMs;
    private Long endSplitMs;
    private boolean active;
    private boolean completed;
    private boolean submissionTriggered;
    private volatile boolean authorized;
    private volatile String status = "";
    private volatile String authorizedLogin = "";
    private volatile String tournamentPhase = "";
    private volatile String tournamentPhaseLabel = "";
    private volatile String matchId = "";
    private volatile String runSessionId = "";
    private volatile String worldFingerprint = "";
    private volatile String seedHash = "";

    void start(Instant startedAtUtc) {
        this.startedAtUtc = startedAtUtc;
        this.wallClockStartMs = System.currentTimeMillis();
        this.wallClockElapsedMs = 0L;
        this.gameplayElapsedMs = 0L;
        this.lastTickMs = 0L;
        this.netherSplitMs = null;
        this.endSplitMs = null;
        this.active = true;
        this.completed = false;
        this.submissionTriggered = false;
        this.authorized = false;
        this.authorizedLogin = "";
        this.tournamentPhase = "";
        this.tournamentPhaseLabel = "";
        this.matchId = "";
        this.runSessionId = "";
        this.worldFingerprint = "";
        this.seedHash = "";
        this.status = "";
    }

    void resume(Instant startedAtUtc, long wallClockElapsedMs, long gameplayElapsedMs, Long netherSplitMs, Long endSplitMs, String phase, String phaseLabel, String matchId, String runSessionId, String worldFingerprint, String seedHash) {
        this.startedAtUtc = startedAtUtc == null ? Instant.now() : startedAtUtc;
        this.wallClockStartMs = Math.max(0L, System.currentTimeMillis() - Math.max(0L, wallClockElapsedMs));
        this.wallClockElapsedMs = Math.max(0L, wallClockElapsedMs);
        this.gameplayElapsedMs = Math.max(0L, gameplayElapsedMs);
        this.lastTickMs = System.currentTimeMillis();
        this.netherSplitMs = netherSplitMs;
        this.endSplitMs = endSplitMs;
        this.active = true;
        this.completed = false;
        this.submissionTriggered = false;
        this.tournamentPhase = phase == null ? "" : phase;
        this.tournamentPhaseLabel = phaseLabel == null || phaseLabel.isBlank() ? this.tournamentPhase : phaseLabel;
        this.matchId = matchId == null ? "" : matchId;
        this.runSessionId = runSessionId == null ? "" : runSessionId;
        this.worldFingerprint = worldFingerprint == null ? "" : worldFingerprint;
        this.seedHash = seedHash == null ? "" : seedHash;
        this.status = "";
        MinecraftSpeedrunModClient modClient = MinecraftSpeedrunModClient.getInstance();
        if (modClient != null) {
            modClient.showPhaseLabelTemporarily();
        }
    }

    void reset(String status) {
        this.startedAtUtc = null;
        this.wallClockStartMs = 0L;
        this.wallClockElapsedMs = 0L;
        this.gameplayElapsedMs = 0L;
        this.lastTickMs = 0L;
        this.netherSplitMs = null;
        this.endSplitMs = null;
        this.active = false;
        this.completed = false;
        this.submissionTriggered = false;
        this.authorized = false;
        this.authorizedLogin = "";
        this.tournamentPhase = "";
        this.tournamentPhaseLabel = "";
        this.matchId = "";
        this.runSessionId = "";
        this.worldFingerprint = "";
        this.seedHash = "";
        this.status = status;
    }

    boolean tick(boolean pausedByMenu, boolean dragonDying) {
        if (!active) {
            return false;
        }

        long now = System.currentTimeMillis();
        if (!completed && lastTickMs != 0L) {
            long delta = Math.max(0L, now - lastTickMs);
            wallClockElapsedMs = Math.max(0L, now - wallClockStartMs);
            if (!pausedByMenu) {
                gameplayElapsedMs += delta;
            }
        } else if (lastTickMs == 0L) {
            wallClockElapsedMs = 0L;
        }
        lastTickMs = now;

        if (completed) {
            return false;
        }

        if (dragonDying && !submissionTriggered) {
            submissionTriggered = true;
            completed = true;
            return true;
        }

        return false;
    }

    Snapshot snapshot() {
        Instant started = startedAtUtc == null ? Instant.now() : startedAtUtc;
        Instant finished = started.plusMillis(wallClockElapsedMs);
        return new Snapshot(
            started,
            finished,
            wallClockElapsedMs,
            gameplayElapsedMs,
            netherSplitMs,
            endSplitMs,
            tournamentPhase,
            matchId,
            runSessionId,
            worldFingerprint,
            seedHash
        );
    }

    boolean isActive() {
        return active;
    }

    boolean isCompleted() {
        return completed;
    }

    boolean shouldRender() {
        return active || completed || !status.isBlank();
    }

    long getWallClockElapsedMs() {
        return wallClockElapsedMs;
    }

    long getGameplayElapsedMs() {
        return gameplayElapsedMs;
    }

    String getStatus() {
        return status;
    }

    boolean isAuthorized() {
        return authorized;
    }

    String getAuthorizedLogin() {
        return authorizedLogin;
    }

    String getTournamentPhaseLabel() {
        return tournamentPhaseLabel;
    }

    void markAuthorized(String twitchLogin, String phase, String phaseLabel, String matchId) {
        this.authorized = true;
        this.authorizedLogin = twitchLogin == null ? "" : twitchLogin;
        this.tournamentPhase = phase == null ? "" : phase;
        this.tournamentPhaseLabel = phaseLabel == null || phaseLabel.isBlank() ? this.tournamentPhase : phaseLabel;
        this.matchId = matchId == null ? "" : matchId;
        this.status = "";
        MinecraftSpeedrunModClient modClient = MinecraftSpeedrunModClient.getInstance();
        if (modClient != null) {
            modClient.showPhaseLabelTemporarily();
        }
    }

    void markAuthorizedBlocked(String twitchLogin, String phase, String phaseLabel, String matchId, String message) {
        markAuthorized(twitchLogin, phase, phaseLabel, matchId);
        this.status = message == null ? "" : message;
    }

    void markAuthorizationFailed(String message) {
        this.authorized = false;
        this.authorizedLogin = "";
        this.status = message == null ? "" : message;
    }

    void setTournamentContext(String phase, String phaseLabel, String matchId) {
        this.tournamentPhase = phase == null ? "" : phase;
        this.tournamentPhaseLabel = phaseLabel == null || phaseLabel.isBlank() ? this.tournamentPhase : phaseLabel;
        this.matchId = matchId == null ? "" : matchId;
        MinecraftSpeedrunModClient modClient = MinecraftSpeedrunModClient.getInstance();
        if (modClient != null) {
            modClient.showPhaseLabelTemporarily();
        }
    }

    void markSubmitted(String statusMessage) {
        this.status = statusMessage != null ? statusMessage : "";
        this.active = false;
    }

    void markSubmitFailed(String message) {
        this.status = message;
    }

    boolean captureNetherSplit() {
        if (!active || completed || netherSplitMs != null) {
            return false;
        }
        netherSplitMs = Math.max(0L, gameplayElapsedMs);
        return true;
    }

    boolean captureEndSplit() {
        if (!active || completed || endSplitMs != null) {
            return false;
        }
        endSplitMs = Math.max(0L, gameplayElapsedMs);
        return true;
    }

    Long getNetherSplitMs() {
        return netherSplitMs;
    }

    Long getEndSplitMs() {
        return endSplitMs;
    }

    Instant getStartedAtUtc() {
        return startedAtUtc;
    }

    void attachRunSession(String runSessionId, String worldFingerprint, String seedHash) {
        this.runSessionId = runSessionId == null ? "" : runSessionId;
        this.worldFingerprint = worldFingerprint == null ? "" : worldFingerprint;
        this.seedHash = seedHash == null ? "" : seedHash;
    }

    record Snapshot(
        Instant startedAtUtc,
        Instant finishedAtUtc,
        long wallClockElapsedMs,
        long gameplayElapsedMs,
        Long netherSplitMs,
        Long endSplitMs,
        String phase,
        String matchId,
        String runSessionId,
        String worldFingerprint,
        String seedHash
    ) {
    }
}
