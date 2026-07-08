package dev.codex.speedrun.timer;

import net.fabricmc.api.ClientModInitializer;
import net.fabricmc.fabric.api.client.event.lifecycle.v1.ClientTickEvents;
import net.fabricmc.fabric.api.client.keybinding.v1.KeyBindingHelper;
import net.fabricmc.fabric.api.client.networking.v1.ClientPlayConnectionEvents;
import net.fabricmc.loader.api.FabricLoader;
import net.fabricmc.loader.api.ModContainer;
import net.minecraft.client.MinecraftClient;
import net.minecraft.client.gui.DrawContext;
import net.minecraft.client.gui.screen.Screen;
import net.minecraft.client.gui.screen.world.LevelLoadingScreen;
import net.minecraft.client.option.KeyBinding;
import net.minecraft.client.gl.RenderPipelines;
import net.minecraft.client.render.Camera;
import net.minecraft.client.resource.language.I18n;
import net.minecraft.client.util.InputUtil;
import net.minecraft.entity.Entity;
import net.minecraft.entity.boss.dragon.EnderDragonEntity;
import net.minecraft.server.MinecraftServer;
import net.minecraft.server.network.ServerPlayerEntity;
import net.minecraft.client.sound.PositionedSoundInstance;
import net.minecraft.sound.SoundEvents;
import net.minecraft.text.Text;
import net.minecraft.util.Identifier;
import net.minecraft.util.WorldSavePath;
import net.minecraft.util.math.BlockPos;
import net.minecraft.util.math.Vec3d;
import net.minecraft.world.Difficulty;
import net.minecraft.world.GameMode;
import net.minecraft.world.World;
import org.lwjgl.glfw.GLFW;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

import java.io.IOException;
import java.nio.file.Path;
import java.security.MessageDigest;
import java.security.NoSuchAlgorithmException;
import java.time.Instant;
import java.time.format.DateTimeParseException;
import java.util.ArrayList;
import java.util.Comparator;
import java.util.HexFormat;
import java.util.List;
import java.util.function.Consumer;



public final class MinecraftSpeedrunModClient implements ClientModInitializer {
    static final Logger LOGGER = LoggerFactory.getLogger("minecraft_speedrun_timer");
    private static final String MOD_ID = "minecraft_speedrun_timer";
    private static final double DEFAULT_FOV_DEGREES = 70.0D;
    private static MinecraftSpeedrunModClient instance;

    private ModConfig config = ModConfig.load();
    private final RunTimerState timerState = new RunTimerState();
    private final ServerApiClient apiClient = new ServerApiClient();
    private final List<AchievementToast> achievementToasts = new ArrayList<>();



    private KeyBinding openConfigScreenKey;
    private boolean pendingRunStart;
    private boolean pendingFreshWorldCreation;
    private boolean waitingForMatchStart;
    private String waitingAuthorizedLogin = "";
    private String waitingPhase = "";
    private String waitingPhaseLabel = "";
    private String waitingMatchId = "";
    private String waitingWorldSeed = "";
    private String expectedWorldSeed = "";
    private long nextMatchReadyPollMs;
    private long phaseLabelShowUntilMs;
    private String currentDimensionId = "";
    private boolean matchLost;
    private long nextRunSessionPollMs;

    @Override
    public void onInitializeClient() {
        assertNoExtraMods();
        instance = this;
        openConfigScreenKey = KeyBindingHelper.registerKeyBinding(new KeyBinding(
            "key.minecraft_speedrun_timer.open_auth",
            InputUtil.Type.KEYSYM,
            GLFW.GLFW_KEY_O,
            KeyBinding.Category.create(Identifier.of("minecraft_speedrun_timer", "general"))
        ));
        ClientPlayConnectionEvents.JOIN.register((handler, sender, client) -> {
            if (!timerState.isActive()) {
                pendingRunStart = true;
                currentDimensionId = "";
            }
            showPhaseLabelTemporarily();
        });
        ClientPlayConnectionEvents.DISCONNECT.register((handler, client) -> {


            pendingRunStart = false;
            pendingFreshWorldCreation = false;
            waitingForMatchStart = false;
            matchLost = false;
            nextRunSessionPollMs = 0L;
            waitingAuthorizedLogin = "";
            waitingPhase = "";
            waitingPhaseLabel = "";
            waitingMatchId = "";
            waitingWorldSeed = "";
            expectedWorldSeed = "";
            nextMatchReadyPollMs = 0L;
            phaseLabelShowUntilMs = 0L;
            currentDimensionId = "";
            persistActiveRunSession();
            timerState.reset(translate("message.minecraft_speedrun_timer.disconnected"));
        });
        ClientTickEvents.END_CLIENT_TICK.register(this::onClientTick);

        if (config.isConfigured()) {
            authorizeConfiguredToken(result -> {
                LOGGER.info("[Auth] Startup token validation complete: ok={}, phase={}", result.ok(), result.phase());
            });
        }
    }

    private static void assertNoExtraMods() {
        List<String> extraMods = FabricLoader.getInstance().getAllMods().stream()
            .map(ModContainer::getMetadata)
            .filter(metadata -> !isAllowedMod(metadata.getId()))
            .map(metadata -> metadata.getId() + " (" + metadata.getName() + ")")
            .sorted(Comparator.naturalOrder())
            .toList();

        if (extraMods.isEmpty()) {
            return;
        }

        LOGGER.error("Remove other mods before launching the tournament mod. Extra mods found: {}", String.join(", ", extraMods));
        throw new IllegalStateException("Remove other mods before launching the tournament mod: " + String.join(", ", extraMods));
    }

    private static boolean isAllowedMod(String id) {
        return id.equals(MOD_ID)
            || id.equals("minecraft")
            || id.equals("java")
            || id.equals("fabricloader")
            || id.equals("fabric-api")
            || id.startsWith("fabric-")
            || id.equals("mixinextras");
    }

    void authorizeConfiguredToken(Consumer<ServerApiClient.ApiResult> callback) {
        apiClient.authorizeAsync(config, result -> {
            MinecraftClient client = MinecraftClient.getInstance();
            if (client == null) {
                applyAuthorizeResult(result);
                callback.accept(result);
                return;
            }
            client.execute(() -> {
                applyAuthorizeResult(result);
                callback.accept(result);
            });
        });
    }

    public void authorizeConfiguredTokenAsync() {
        if (config.isConfigured() && !isAuthorized() && !isWaitingForMatchStart()) {
            authorizeConfiguredToken(result -> {});
        }
    }

    String getHudStatus() {
        return timerState.getStatus();
    }

    public boolean isAuthorized() {
        return config.isConfigured() && timerState.isAuthorized();
    }

    public boolean isReadyToStartRun() {
        return isAuthorized() && timerState.getStatus().isBlank();
    }

    public String tokenButtonTranslationKey() {
        if (!config.isConfigured()) {
            return "button.minecraft_speedrun_timer.mod_token";
        }
        if (timerState.isAuthorized()) {
            return "button.minecraft_speedrun_timer.mod_token_authorized";
        }
        return "button.minecraft_speedrun_timer.mod_token_saved";
    }

    String getAuthorizedLogin() {
        return timerState.getAuthorizedLogin();
    }

    public ModConfig getConfig() {
        return config;
    }

    void reloadConfig() {
        this.config = ModConfig.load();
    }

    public boolean isConfigured() {
        return config.isConfigured();
    }

    public String validateSeedBeforeWorldCreate(String seed) {
        String enteredSeed = seed == null ? "" : seed.trim();
        String requiredSeed = expectedWorldSeed == null ? "" : expectedWorldSeed.trim();
        if (!requiredSeed.isBlank()) {
            if (enteredSeed.isBlank()) {
                return translate("message.minecraft_speedrun_timer.match_seed_required", requiredSeed);
            }
            if (!enteredSeed.equals(requiredSeed)) {
                return translate("message.minecraft_speedrun_timer.match_seed_mismatch", requiredSeed);
            }
            return "";
        }
        if (!enteredSeed.isBlank()) {
            return translate("message.minecraft_speedrun_timer.qualification_seed_forbidden");
        }
        return "";
    }

    public void markFreshWorldCreateApproved() {
        pendingFreshWorldCreation = true;
    }

    public void showPhaseLabelTemporarily() {
        this.phaseLabelShowUntilMs = System.currentTimeMillis() + 10000L;
    }

    public static String translate(String key, Object... args) {
        return I18n.translate(key, args);
    }

    public void openAuthScreen(Screen parent, String message) {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null) {
            return;
        }
        client.setScreen(new AuthScreen(parent, config, this, message));
    }

    public void openMessageScreen(Screen parent, String message) {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null) {
            return;
        }
        client.setScreen(new ModMessageScreen(parent, message));
    }

    public static MinecraftSpeedrunModClient getInstance() {
        return instance;
    }

    public boolean isWaitingForMatchStart() {
        return waitingForMatchStart;
    }

    public boolean isMatchLost() {
        return matchLost;
    }

    public boolean shouldBlockCommandBeforeDragonKill(String command) {
        String normalized = command == null ? "" : command.trim().split("\\s+", 2)[0];
        if (normalized.startsWith("/")) {
            normalized = normalized.substring(1);
        }
        if (ModBuildConfig.DEV_MODE || config.isAllowCreative()) return false;
        return waitingForMatchStart || (timerState.isActive() && !timerState.isCompleted());
    }

    public void showCommandBlockedMessage() {
        sendClientMessage("РљРѕРјР°РЅРґС‹ РґРѕСЃС‚СѓРїРЅС‹ С‚РѕР»СЊРєРѕ РїРѕСЃР»Рµ СѓР±РёР№СЃС‚РІР° РґСЂР°РєРѕРЅР°.");
    }

    private void sendClientMessage(String message) {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client != null && client.player != null) {
            client.player.sendMessage(Text.literal(message), false);
        }
    }

    private void onWorldJoined(MinecraftClient client) {
        matchLost = false;
        nextRunSessionPollMs = 0L;
        if (timerState.isActive()) {
            return;
        }
        if (client.getServer() == null) {
            timerState.reset(translate("message.minecraft_speedrun_timer.singleplayer_only"));
            return;
        }

        String worldKey = currentWorldKey(client);
        if (worldKey.isBlank()) {
            timerState.reset(translate("message.minecraft_speedrun_timer.world_unknown"));
            return;
        }
        if (!pendingFreshWorldCreation) {
            if (tryResumeExistingRunSession(client, worldKey)) {
                return;
            }
            timerState.reset(translate("message.minecraft_speedrun_timer.must_create_new_world"));
            return;
        }
        pendingFreshWorldCreation = false;
        config.markWorldUsed(worldKey);
        if (!saveConfigAfterWorldLock()) {
            timerState.reset(translate("message.minecraft_speedrun_timer.world_lock_failed"));
            return;
        }

        if (!ModBuildConfig.DEV_MODE && !config.isAllowCreative()) {
            if (client.getServer().getSaveProperties().areCommandsAllowed()) {
                timerState.reset(translate("message.minecraft_speedrun_timer.cheats_enabled"));
                return;
            }
            String worldRulesError = validateWorldRules(client);
            if (!worldRulesError.isBlank()) {
                timerState.reset(worldRulesError);
                return;
            }
        }

        if (!config.isConfigured()) {
            timerState.markAuthorizationFailed(translate("message.minecraft_speedrun_timer.token_required"));
            return;
        }

        currentDimensionId = currentDimension(client);
        timerState.start(Instant.now());
        captureInitialSplits();
        authorizeRunStartAndCreateSession(client, worldKey);
    }

    private void onClientTick(MinecraftClient client) {


        while (openConfigScreenKey.wasPressed()) {
            client.setScreen(new HudConfigScreen(client.currentScreen, config, this));
        }

        if (client.world == null || client.player == null) {
            return;
        }

        if (pendingRunStart) {
            pendingRunStart = false;
            onWorldJoined(client);
        }

        if (waitingForMatchStart) {
            pollMatchReady(client);
            return;
        }

        if (!timerState.isActive()) {
            return;
        }

        if (!timerState.isCompleted()) {
            RunTimerState.Snapshot snap = timerState.snapshot();
            if (!snap.matchId().isBlank() && !snap.runSessionId().isBlank() && !matchLost) {
                long now = System.currentTimeMillis();
                if (now >= nextRunSessionPollMs) {
                    nextRunSessionPollMs = now + 5000L;
                    apiClient.validateRunSessionAsync(config, snap.runSessionId(), snap.worldFingerprint(), snap.seedHash(), result -> client.execute(() -> {
                        if (!result.ok()) {
                            if ("match_lost".equals(result.tournamentReason())) {
                                matchLost = true;
                                clearPersistedRunSession();
                                timerState.reset("Р’С‹ РІС‚РѕСЂРѕР№!");
                                sendClientMessage("Р’С‹ РїСЂРѕРёРіСЂР°Р»Рё! РЎРѕРїРµСЂРЅРёРє СѓР¶Рµ С„РёРЅРёС€РёСЂРѕРІР°Р».");
                            }
                        }
                    }));
                }
            }
        }
        if (!ModBuildConfig.DEV_MODE && !config.isAllowCreative()) {
            String worldRulesError = validateWorldRules(client);
            if (!worldRulesError.isBlank()) {
                timerState.reset(worldRulesError);
                return;
            }
        }

        boolean pausedByMenu = client.currentScreen != null
            && (client.currentScreen.shouldPause() || client.currentScreen instanceof LevelLoadingScreen);
        boolean inEnd = World.END.getValue().toString().equals(currentDimension(client));
        boolean dragonDying = inEnd && hasDragonWithZeroHealth(client);
        boolean dragonKilled = timerState.tick(pausedByMenu, dragonDying);
        captureSplitTransitions(client);

        if (dragonKilled) {
            submitRun();
        }
    }

    public void renderHudOverlay(DrawContext context) {
        MinecraftClient client = MinecraftClient.getInstance();
        if (!shouldRenderHudOverlay(client)) {
            return;
        }
        if (client.currentScreen instanceof HudConfigScreen) {
            return;
        }

        renderCoordinatesOverlay(client, context);
        renderAchievementToasts(client, context);

        renderTimerOverlay(context, client, resolveHudX(context.getScaledWindowWidth()), resolveHudY(context.getScaledWindowHeight()));
    }

    public void renderHudPositionPreview(DrawContext context, int x, int y) {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null) {
            return;
        }

        if (shouldRenderHudOverlay(client) && timerState.shouldRender()) {
            renderTimerOverlay(context, client, x, y);
            return;
        }

        context.drawTextWithShadow(client.textRenderer, Text.translatable("hud.minecraft_speedrun_timer.gameplay", "00:12.345"), x, y, 0xFF80FF80);
        context.drawTextWithShadow(client.textRenderer, Text.translatable("hud.minecraft_speedrun_timer.real_time", "00:15.678"), x, y + 12, 0xFFFFFFFF);
    }

    private boolean shouldRenderHudOverlay(MinecraftClient client) {
        return client != null
            && client.world != null
            && client.player != null
            && !client.options.hudHidden;
    }

    public int resolveHudX(int scaledWindowWidth) {
        return resolveHudAxis(config.getHudX(), config.getHudReferenceWidth(), scaledWindowWidth);
    }

    public int resolveHudY(int scaledWindowHeight) {
        return resolveHudAxis(config.getHudY(), config.getHudReferenceHeight(), scaledWindowHeight);
    }

    private int resolveHudAxis(int value, int referenceSize, int currentSize) {
        if (currentSize <= 0) {
            return value;
        }

        int safeReference = referenceSize;
        if (safeReference <= 0) {
            MinecraftClient client = MinecraftClient.getInstance();
            double scaleFactor = client == null || client.getWindow() == null ? 2.0D : client.getWindow().getScaleFactor();
            safeReference = (int) Math.max(1.0D, Math.round(currentSize * scaleFactor / 2.0D));
        }

        int resolved = (int) Math.round(value * (currentSize / (double) safeReference));
        return Math.max(0, Math.min(currentSize - 1, resolved));
    }

    private void renderTimerOverlay(DrawContext context, MinecraftClient client, int x, int y) {
        if (!timerState.shouldRender()) {
            return;
        }

        context.drawTextWithShadow(client.textRenderer, Text.translatable("hud.minecraft_speedrun_timer.gameplay", formatDuration(timerState.getGameplayElapsedMs())), x, y, 0xFF80FF80);
        context.drawTextWithShadow(client.textRenderer, Text.translatable("hud.minecraft_speedrun_timer.real_time", formatDuration(timerState.getWallClockElapsedMs())), x, y + 12, 0xFFFFFFFF);
        
        long now = System.currentTimeMillis();
        boolean showPhase = now < phaseLabelShowUntilMs && !timerState.getTournamentPhaseLabel().isBlank();
        if (showPhase) {
            long remaining = phaseLabelShowUntilMs - now;
            long elapsed = 10000L - remaining;
            int alpha = 255;
            if (elapsed < 1000L) { // Fade-in during first 1 sec
                alpha = (int) (255 * elapsed / 1000L);
            } else if (remaining < 2000L) { // Fade-out during last 2 secs
                alpha = (int) (255 * remaining / 2000L);
            }
            int color = (alpha << 24) | 0xC084FC;
            context.drawTextWithShadow(client.textRenderer, Text.literal("Р­С‚Р°Рї: " + timerState.getTournamentPhaseLabel()), x, y + 24, color);
        }
        
        if (!timerState.getStatus().isBlank()) {
            int statusY = timerState.getTournamentPhaseLabel().isBlank() ? y + 24 : y + 36;
            context.drawTextWithShadow(client.textRenderer, Text.literal(timerState.getStatus()), x, statusY, 0xFFFFD37F);
        }
    }

    private void renderCoordinatesOverlay(MinecraftClient client, DrawContext context) {
        if (!config.isCoordinatesEnabled() || client.player == null) {
            return;
        }

        BlockPos pos = client.player.getBlockPos();
        String xLabel = "X ";
        String xValue = Integer.toString(pos.getX());
        String yLabel = " Y ";
        String yValue = Integer.toString(pos.getY());
        String zLabel = " Z ";
        String zValue = Integer.toString(pos.getZ());
        String eLabel = " E ";
        String eValue = entityCounter(client);
        int width = client.textRenderer.getWidth(xLabel)
            + client.textRenderer.getWidth(xValue)
            + client.textRenderer.getWidth(yLabel)
            + client.textRenderer.getWidth(yValue)
            + client.textRenderer.getWidth(zLabel)
            + client.textRenderer.getWidth(zValue)
            + client.textRenderer.getWidth(eLabel)
            + client.textRenderer.getWidth(eValue);
        int x = (context.getScaledWindowWidth() - width) / 2;
        int y = context.getScaledWindowHeight() - config.getCoordinatesOffsetY();
        x = drawCoordinatePart(context, client, xLabel, x, y, 0xFFFFD84D);
        x = drawCoordinatePart(context, client, xValue, x, y, 0xFFFFFFFF);
        x = drawCoordinatePart(context, client, yLabel, x, y, 0xFFFFD84D);
        x = drawCoordinatePart(context, client, yValue, x, y, 0xFFFFFFFF);
        x = drawCoordinatePart(context, client, zLabel, x, y, 0xFFFFD84D);
        x = drawCoordinatePart(context, client, zValue, x, y, 0xFFFFFFFF);
        x = drawCoordinatePart(context, client, eLabel, x, y, 0xFFFFD84D);
        drawCoordinatePart(context, client, eValue, x, y, 0xFFFFFFFF);
    }

    private String entityCounter(MinecraftClient client) {
        if (client.world == null || client.player == null) {
            return "0/0";
        }

        Camera camera = client.gameRenderer.getCamera();
        Entity focusedEntity = camera.getFocusedEntity();
        Vec3d cameraPos = focusedEntity == null
            ? client.player.getCameraPosVec(camera.getLastTickProgress())
            : focusedEntity.getCameraPosVec(camera.getLastTickProgress());
        Vec3d forward = Vec3d.fromPolar(camera.getPitch(), camera.getYaw()).normalize();
        double dotThreshold = visibleDotThreshold(client);

        int visible = 0;
        int loaded = 0;
        for (Entity entity : client.world.getEntities()) {
            if (!shouldCountLoadedEntity(entity)) {
                continue;
            }
            loaded++;
            if (shouldCountVisibleEntity(client, camera, cameraPos, forward, dotThreshold, entity)) {
                visible++;
            }
        }
        return visible + "/" + loaded;
    }

    private boolean shouldCountLoadedEntity(Entity entity) {
        return entity != null && !entity.isRemoved();
    }

    private boolean shouldCountVisibleEntity(MinecraftClient client, Camera camera, Vec3d cameraPos, Vec3d forward, double dotThreshold, Entity entity) {
        if (entity == client.player && !camera.isThirdPerson()) {
            return false;
        }

        Vec3d toEntity = entity.getBoundingBox().getCenter().subtract(cameraPos);
        if (toEntity.lengthSquared() < 0.0001D) {
            return true;
        }

        return forward.dotProduct(toEntity.normalize()) >= dotThreshold;
    }

    private double visibleDotThreshold(MinecraftClient client) {
        double verticalFov = DEFAULT_FOV_DEGREES;
        if (client.options != null && client.options.getFov() != null) {
            verticalFov = client.options.getFov().getValue();
        }

        double framebufferWidth = client.getWindow() == null ? 16.0D : Math.max(1, client.getWindow().getFramebufferWidth());
        double framebufferHeight = client.getWindow() == null ? 9.0D : Math.max(1, client.getWindow().getFramebufferHeight());
        double aspectRatio = framebufferWidth / framebufferHeight;
        double halfVerticalFov = Math.toRadians(Math.max(30.0D, Math.min(110.0D, verticalFov)) / 2.0D);
        double halfHorizontalFov = Math.atan(Math.tan(halfVerticalFov) * aspectRatio);
        double halfDiagonalFov = Math.atan(Math.hypot(Math.tan(halfVerticalFov), Math.tan(halfHorizontalFov)));
        return Math.cos(halfDiagonalFov);
    }

    private int drawCoordinatePart(DrawContext context, MinecraftClient client, String text, int x, int y, int color) {
        context.drawTextWithShadow(client.textRenderer, Text.literal(text), x, y, color);
        return x + client.textRenderer.getWidth(text);
    }

    private void renderAchievementToasts(MinecraftClient client, DrawContext context) {
        if (achievementToasts.isEmpty()) {
            return;
        }

        long now = System.currentTimeMillis();
        achievementToasts.removeIf(toast -> now - toast.startedAtMs > AchievementToast.LIFETIME_MS);

        int width = Math.min(178, context.getScaledWindowWidth() - 12);
        int height = 34;
        int x = context.getScaledWindowWidth() - width - 6;
        int y = context.getScaledWindowHeight() - height - 6;
        int rendered = 0;
        for (int i = achievementToasts.size() - 1; i >= 0 && rendered < 2; i--) {
            AchievementToast toast = achievementToasts.get(i);
            int toastY = y - rendered * (height + 4);
            context.fill(x, toastY, x + width, toastY + height, 0xE6130C20);
            context.fill(x, toastY, x + 2, toastY + height, 0xFFC084FC);
            Identifier icon = achievementIcon(toast.slug);
            if (icon == null) {
                context.fill(x + 7, toastY + 7, x + 27, toastY + 27, 0xFF24113F);
                context.fill(x + 12, toastY + 13, x + 22, toastY + 19, 0xFF8EF0BA);
            } else {
                context.drawTexture(RenderPipelines.GUI_TEXTURED, icon, x + 6, toastY + 5, 0.0F, 0.0F, 24, 24, 64, 64, 64, 64);
            }
            context.drawTextWithShadow(client.textRenderer, Text.literal("Р”РѕСЃС‚РёР¶РµРЅРёРµ"), x + 34, toastY + 5, 0xFFC084FC);
            context.drawTextWithShadow(client.textRenderer, Text.literal(trimToWidth(client, toast.name, width - 42)), x + 34, toastY + 18, 0xFFFFFFFF);
            rendered++;
        }
    }

    private Identifier achievementIcon(String slug) {
        if (slug == null || slug.isBlank()) {
            return null;
        }
        return switch (slug) {
            case "legend_return", "first_dragon", "diligent", "hardened", "machine", "top_three_days", "rock_bottom", "seconds_hunter", "marathoner", "stability", "on_flow" ->
                Identifier.of(MOD_ID, "textures/achievement/" + slug + ".png");
            default -> null;
        };
    }

    private String trimToWidth(MinecraftClient client, String value, int maxWidth) {
        if (client.textRenderer.getWidth(value) <= maxWidth) {
            return value;
        }
        String suffix = "...";
        int suffixWidth = client.textRenderer.getWidth(suffix);
        StringBuilder builder = new StringBuilder(value);
        while (!builder.isEmpty() && client.textRenderer.getWidth(builder.toString()) + suffixWidth > maxWidth) {
            builder.deleteCharAt(builder.length() - 1);
        }
        return builder + suffix;
    }

    private void captureSplitTransitions(MinecraftClient client) {
        String dimensionId = currentDimension(client);
        if (dimensionId.isBlank()) {
            return;
        }
        if (currentDimensionId.isBlank()) {
            currentDimensionId = dimensionId;
            return;
        }
        if (dimensionId.equals(currentDimensionId)) {
            return;
        }

        if (World.NETHER.getValue().toString().equals(dimensionId)) {
            if (timerState.captureNetherSplit()) {
                String timeStr = formatDuration(timerState.getNetherSplitMs());
                sendClientMessage(translate("message.minecraft_speedrun_timer.split_nether", timeStr));
                playSplitSound();
            }
        }
        if (World.END.getValue().toString().equals(dimensionId)) {
            if (timerState.captureEndSplit()) {
                String timeStr = formatDuration(timerState.getEndSplitMs());
                sendClientMessage(translate("message.minecraft_speedrun_timer.split_end", timeStr));
                playSplitSound();
            }
        }

        currentDimensionId = dimensionId;
        persistActiveRunSession();
    }

    private boolean hasDragonWithZeroHealth(MinecraftClient client) {
        for (Entity entity : client.world.getEntities()) {
            if (entity instanceof EnderDragonEntity dragon && dragon.getHealth() <= 0) {
                return true;
            }
        }
        return false;
    }

    private String currentDimension(MinecraftClient client) {
        if (client.world == null) {
            return "";
        }
        return client.world.getRegistryKey().getValue().toString();
    }

    private String currentWorldKey(MinecraftClient client) {
        if (client.getServer() == null) {
            return "";
        }
        Path path = client.getServer().getSavePath(WorldSavePath.ROOT).toAbsolutePath().normalize();
        return path.toString();
    }

    private boolean saveConfigAfterWorldLock() {
        try {
            config.save();
            return true;
        } catch (IOException exception) {
            LOGGER.error("Failed to save used world lock.", exception);
            return false;
        }
    }

    private void submitRun() {
        if (!timerState.isAuthorized()) {
            timerState.markSubmitFailed(translate("message.minecraft_speedrun_timer.submit_not_authorized"));
            return;
        }

        RunTimerState.Snapshot snapshot = timerState.snapshot();
        apiClient.submitRunAsync(config, snapshot, result -> MinecraftClient.getInstance().execute(() -> {
            if (result.ok()) {
                String successMsg = translate("message.minecraft_speedrun_timer.submit_success");
                timerState.markSubmitted(successMsg);
                sendClientMessage(successMsg);
                clearPersistedRunSession();
                for (ServerApiClient.AchievementUnlock achievement : result.achievements()) {
                    showAchievementToast(achievement);
                }
            } else {
                timerState.markSubmitFailed(result.message());
            }
        }));
    }

    private void pollMatchReady(MinecraftClient client) {
        long now = System.currentTimeMillis();
        if (now < nextMatchReadyPollMs || waitingMatchId.isBlank()) {
            return;
        }
        nextMatchReadyPollMs = now + 1000L;
        apiClient.markMatchReadyAsync(config, waitingMatchId, result -> client.execute(() -> applyMatchReadyResult(result)));
    }

    private void showAchievementToast(ServerApiClient.AchievementUnlock achievement) {
        MinecraftClient client = MinecraftClient.getInstance();
        achievementToasts.add(new AchievementToast(achievement));
        if (achievementToasts.size() > 3) {
            achievementToasts.remove(0);
        }
        if (client != null) {
            client.getSoundManager().play(PositionedSoundInstance.ui(SoundEvents.UI_TOAST_CHALLENGE_COMPLETE, 1.0F));
        }
    }

    private void playSplitSound() {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client != null) {
            client.getSoundManager().play(PositionedSoundInstance.ui(SoundEvents.ENTITY_EXPERIENCE_ORB_PICKUP, 1.0F));
        }
    }

    private void applyAuthorizeResult(ServerApiClient.ApiResult result) {
        if (result.ok()) {
            expectedWorldSeed = result.worldSeed() == null ? "" : result.worldSeed().trim();
            timerState.setTournamentContext(result.phase(), result.phaseLabel(), result.matchId());
            if (!result.canStartOfficialRun()) {
                if (!result.matchId().isBlank()) {
                    enterMatchWaiting(result, result.message());
                    return;
                }
                timerState.markAuthorizedBlocked(
                    result.message(),
                    result.phase(),
                    result.phaseLabel(),
                    result.matchId(),
                    result.tournamentReason()
                );
                return;
            }
            String seedError = validateCurrentWorldSeed(result.worldSeed());
            if (!seedError.isBlank()) {
                waitingForMatchStart = false;
                timerState.reset(seedError);
                return;
            }
            waitingForMatchStart = false;
            timerState.markAuthorized(result.message(), result.phase(), result.phaseLabel(), result.matchId());
            return;
        }
        expectedWorldSeed = "";
        timerState.markAuthorizationFailed(result.message());
    }

    private void applyMatchReadyResult(ServerApiClient.ApiResult result) {
        if (!result.ok()) {
            timerState.markAuthorizationFailed(result.message());
            return;
        }
        if (!result.canStartOfficialRun()) {
            enterMatchWaiting(result, waitingAuthorizedLogin);
            return;
        }
        waitingForMatchStart = false;
        startOfficialMatchRun(result);
    }

    private void enterMatchWaiting(ServerApiClient.ApiResult result, String authorizedLogin) {
        waitingForMatchStart = true;
        waitingAuthorizedLogin = authorizedLogin == null ? "" : authorizedLogin;
        waitingPhase = result.phase();
        waitingPhaseLabel = result.phaseLabel();
        waitingMatchId = result.matchId();
        waitingWorldSeed = result.worldSeed();
        expectedWorldSeed = waitingWorldSeed == null ? "" : waitingWorldSeed.trim();
        nextMatchReadyPollMs = System.currentTimeMillis() + 3000L;

        timerState.reset(matchWaitingStatus());
        timerState.setTournamentContext(waitingPhase, waitingPhaseLabel, waitingMatchId);
    }

    private String matchWaitingStatus() {
        if (waitingWorldSeed == null || waitingWorldSeed.isBlank()) {
            return translate("message.minecraft_speedrun_timer.match_not_started");
        }
        return translate("message.minecraft_speedrun_timer.match_waiting_with_seed", waitingWorldSeed);
    }

    private void startOfficialMatchRun(ServerApiClient.ApiResult result) {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null || client.world == null || client.player == null) {
            return;
        }

        String requiredSeed = result.worldSeed().isBlank() ? waitingWorldSeed : result.worldSeed();
        String seedError = validateCurrentWorldSeed(requiredSeed);
        if (!seedError.isBlank()) {
            waitingForMatchStart = false;
            timerState.reset(seedError);
            return;
        }

        currentDimensionId = currentDimension(client);
        timerState.start(Instant.now());
        if (World.NETHER.getValue().toString().equals(currentDimensionId)) {
            timerState.captureNetherSplit();
        }
        if (World.END.getValue().toString().equals(currentDimensionId)) {
            timerState.captureEndSplit();
        }

        String phase = result.phase().isBlank() ? waitingPhase : result.phase();
        String phaseLabel = result.phaseLabel().isBlank() ? waitingPhaseLabel : result.phaseLabel();
        String matchId = result.matchId().isBlank() ? waitingMatchId : result.matchId();
        timerState.markAuthorized(waitingAuthorizedLogin, phase, phaseLabel, matchId);
        startServerRunSession(client, currentWorldKey(client));
    }

    private boolean tryResumeExistingRunSession(MinecraftClient client, String worldKey) {
        ModConfig.ActiveRunSession session = config.getActiveRunSession();
        String seedHash = currentWorldSeedHash(client);
        if (!session.matchesWorld(worldKey, seedHash)) {
            return false;
        }
        long nowMs = System.currentTimeMillis();
        if (session.isExpired(nowMs)) {
            clearPersistedRunSession();
            timerState.reset(translate("message.minecraft_speedrun_timer.run_session_expired"));
            return true;
        }
        long startedAtMs = session.startedAtMs() > 0L ? session.startedAtMs() : nowMs;
        long elapsedMs = Math.max(0L, nowMs - startedAtMs);
        Long netherSplit = session.netherSplitMs() >= 0L ? session.netherSplitMs() : null;
        Long endSplit = session.endSplitMs() >= 0L ? session.endSplitMs() : null;
        timerState.resume(
            Instant.ofEpochMilli(startedAtMs),
            elapsedMs,
            session.gameplayElapsedMs(),
            netherSplit,
            endSplit,
            session.phase(),
            session.phase(),
            session.matchId(),
            session.id(),
            session.worldFingerprint(),
            session.seedHash()
        );
        currentDimensionId = currentDimension(client);
        apiClient.validateRunSessionAsync(config, session.id(), session.worldFingerprint(), session.seedHash(), result -> client.execute(() -> {
            if (!result.ok()) {
                if ("match_lost".equals(result.tournamentReason())) {
                    matchLost = true;
                    clearPersistedRunSession();
                    timerState.reset("Р’С‹ РІС‚РѕСЂРѕР№!");
                    sendClientMessage("Р’С‹ РїСЂРѕРёРіСЂР°Р»Рё! РЎРѕРїРµСЂРЅРёРє СѓР¶Рµ С„РёРЅРёС€РёСЂРѕРІР°Р».");
                    return;
                }
                clearPersistedRunSession();
                timerState.reset(result.message());
                return;
            }
            timerState.markAuthorized(result.message(), result.phase(), result.phaseLabel(), result.matchId());
            persistActiveRunSession(result.runSessionExpiresAt());
        }));
        return true;
    }

    private void startServerRunSession(MinecraftClient client, String worldKey) {
        String seedHash = currentWorldSeedHash(client);
        apiClient.createRunSessionAsync(config, worldKey, seedHash, result -> client.execute(() -> {
            if (!result.ok()) {
                clearPersistedRunSession();
                timerState.reset(result.message());
                return;
            }
            expectedWorldSeed = result.worldSeed() == null ? "" : result.worldSeed().trim();
            timerState.setTournamentContext(result.phase(), result.phaseLabel(), result.matchId());
            timerState.markAuthorized(result.message(), result.phase(), result.phaseLabel(), result.matchId());
            timerState.attachRunSession(result.runSessionId(), worldKey, seedHash);
            persistActiveRunSession(result.runSessionExpiresAt());
        }));
    }

    private void authorizeRunStartAndCreateSession(MinecraftClient client, String worldKey) {
        authorizeConfiguredToken(result -> {
            if (!result.ok() || !result.canStartOfficialRun() || !timerState.isActive()) {
                return;
            }
            startServerRunSession(client, worldKey);
        });
    }

    private void captureInitialSplits() {
        if (World.NETHER.getValue().toString().equals(currentDimensionId)) {
            timerState.captureNetherSplit();
        }
        if (World.END.getValue().toString().equals(currentDimensionId)) {
            timerState.captureEndSplit();
        }
    }

    private void persistActiveRunSession() {
        persistActiveRunSession("");
    }

    private void persistActiveRunSession(String expiresAt) {
        RunTimerState.Snapshot snapshot = timerState.snapshot();
        if (snapshot.runSessionId().isBlank() || snapshot.worldFingerprint().isBlank() || snapshot.seedHash().isBlank()) {
            return;
        }
        long expiresAtMs = parseInstantMs(expiresAt);
        if (expiresAtMs <= 0L) {
            expiresAtMs = config.getActiveRunSession().expiresAtMs();
        }
        long netherSplit = timerState.getNetherSplitMs() == null ? -1L : timerState.getNetherSplitMs();
        long endSplit = timerState.getEndSplitMs() == null ? -1L : timerState.getEndSplitMs();
        config.setActiveRunSession(new ModConfig.ActiveRunSession(
            snapshot.runSessionId(),
            snapshot.worldFingerprint(),
            snapshot.seedHash(),
            snapshot.startedAtUtc().toEpochMilli(),
            expiresAtMs,
            snapshot.phase(),
            snapshot.matchId(),
            netherSplit,
            endSplit,
            System.currentTimeMillis(),
            timerState.getGameplayElapsedMs()
        ));
        saveConfigAfterWorldLock();
    }

    private void clearPersistedRunSession() {
        config.clearActiveRunSession();
        saveConfigAfterWorldLock();
    }

    private String validateCurrentWorldSeed(String requiredSeed) {
        String expectedSeed = requiredSeed == null ? "" : requiredSeed.trim();
        if (expectedSeed.isBlank()) {
            return "";
        }

        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null || client.world == null || client.player == null) {
            return "";
        }

        String actualSeed = currentWorldSeed(client);
        if (actualSeed.isBlank()) {
            return translate("message.minecraft_speedrun_timer.world_seed_unknown");
        }
        if (!actualSeed.equals(expectedSeed)) {
            return translate("message.minecraft_speedrun_timer.world_seed_mismatch", expectedSeed);
        }
        return "";
    }

    private String validateWorldRules(MinecraftClient client) {
        MinecraftServer server = client.getServer();
        if (server == null || client.player == null) {
            return "";
        }
        if (server.getSaveProperties().getDifficulty() != Difficulty.NORMAL) {
            return translate("message.minecraft_speedrun_timer.difficulty_must_be_normal");
        }
        if (client.interactionManager != null && client.interactionManager.getCurrentGameMode() != GameMode.SURVIVAL) {
            return translate("message.minecraft_speedrun_timer.survival_required");
        }
        return "";
    }

    private String currentWorldSeed(MinecraftClient client) {
        MinecraftServer server = client.getServer();
        if (server == null) {
            return "";
        }
        return Long.toString(server.getSaveProperties().getGeneratorOptions().getSeed());
    }

    private String currentWorldSeedHash(MinecraftClient client) {
        return sha256Hex(currentWorldSeed(client));
    }

    private static long parseInstantMs(String value) {
        if (value == null || value.isBlank()) {
            return 0L;
        }
        try {
            return Instant.parse(value).toEpochMilli();
        } catch (DateTimeParseException exception) {
            return 0L;
        }
    }

    private static String sha256Hex(String value) {
        try {
            MessageDigest digest = MessageDigest.getInstance("SHA-256");
            return HexFormat.of().formatHex(digest.digest((value == null ? "" : value).getBytes(java.nio.charset.StandardCharsets.UTF_8)));
        } catch (NoSuchAlgorithmException exception) {
            throw new IllegalStateException("SHA-256 unavailable", exception);
        }
    }

    private static String formatDuration(long millis) {
        if (millis <= 0L) {
            return "00:00.000";
        }
        long totalSeconds = millis / 1000L;
        long totalMinutes = totalSeconds / 60L;
        long hours = totalMinutes / 60L;
        long minutes = totalMinutes % 60L;
        long seconds = totalSeconds % 60L;
        long milliseconds = millis % 1000L;
        if (hours > 0) {
            return String.format("%d:%02d:%02d.%03d", hours, minutes, seconds, milliseconds);
        } else {
            return String.format("%02d:%02d.%03d", minutes, seconds, milliseconds);
        }
    }

    private static final class AchievementToast {
        private static final long LIFETIME_MS = 5200L;

        private final String slug;
        private final String name;
        private final long startedAtMs;

        private AchievementToast(ServerApiClient.AchievementUnlock achievement) {
            this.slug = achievement == null ? "" : achievement.slug();
            String achievementName = achievement == null ? "" : achievement.name();
            this.name = achievementName == null || achievementName.isBlank() ? "Р”РѕСЃС‚РёР¶РµРЅРёРµ" : achievementName;
            this.startedAtMs = System.currentTimeMillis();
        }
    }


}
