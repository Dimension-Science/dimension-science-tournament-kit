package dev.codex.speedrun.timer;

import net.minecraft.client.MinecraftClient;
import net.minecraft.client.gui.DrawContext;
import net.minecraft.client.gui.screen.Screen;
import net.minecraft.client.gui.widget.ButtonWidget;
import net.minecraft.client.gui.widget.TextFieldWidget;
import net.minecraft.text.Text;

import net.minecraft.client.gl.RenderPipelines;
import net.minecraft.util.Identifier;

import java.io.IOException;

final class AuthScreen extends Screen {
    private static final Identifier LOGO_TEXTURE = Identifier.of("minecraft_speedrun_timer", "textures/gui/logo.png");

    private final Screen parent;
    private final ModConfig config;
    private final MinecraftSpeedrunModClient modClient;
    private TextFieldWidget modTokenField;
    private ButtonWidget tokenVisibilityButton;
    private boolean tokenVisible;
    private String message;

    AuthScreen(Screen parent, ModConfig config, MinecraftSpeedrunModClient modClient) {
        this(parent, config, modClient, null);
    }

    AuthScreen(Screen parent, ModConfig config, MinecraftSpeedrunModClient modClient, String initialMessage) {
        super(Text.translatable("screen.minecraft_speedrun_timer.auth"));
        this.parent = parent;
        this.config = config;
        this.modClient = modClient;
        this.message = initialMessage == null || initialMessage.isBlank() ? modClient.getHudStatus() : initialMessage;
    }

    @Override
    protected void init() {
        int left = this.width / 2 - 140;
        int centerY = this.height / 2 - 50;

        modTokenField = new TextFieldWidget(this.textRenderer, left, centerY + 28, 250, 20, Text.translatable("field.minecraft_speedrun_timer.mod_token"));
        modTokenField.setMaxLength(256);
        modTokenField.setText(config.getModToken());
        addDrawableChild(modTokenField);

        tokenVisibilityButton = addDrawableChild(ButtonWidget.builder(tokenVisibilityText(), button -> toggleTokenVisibility())
            .dimensions(left + 256, centerY + 28, 24, 20)
            .build());
        addDrawableChild(ButtonWidget.builder(Text.translatable("button.minecraft_speedrun_timer.save_and_authorize"), button -> saveAndAuthorize())
            .dimensions(left, centerY + 70, 280, 20)
            .build());
        addDrawableChild(ButtonWidget.builder(Text.translatable("button.minecraft_speedrun_timer.close"), button -> close())
            .dimensions(left, centerY + 98, 280, 20)
            .build());

        setInitialFocus(modTokenField);
    }

    @Override
    public void close() {
        MinecraftClient.getInstance().setScreen(parent);
    }

    @Override
    public void render(DrawContext context, int mouseX, int mouseY, float delta) {
        super.render(context, mouseX, mouseY, delta);
        int left = this.width / 2 - 140;
        int centerY = this.height / 2 - 50;

        renderHiddenTokenOverlay(context, left, centerY + 28);
        context.drawTexture(RenderPipelines.GUI_TEXTURED, LOGO_TEXTURE, this.width / 2 - 16, centerY - 68, 0.0F, 0.0F, 32, 32, 64, 64, 64, 64);
        context.drawCenteredTextWithShadow(this.textRenderer, this.title, this.width / 2, centerY - 28, 0xFFFFFF);
        context.drawTextWithShadow(this.textRenderer, Text.translatable("label.minecraft_speedrun_timer.site", ModConfig.OFFICIAL_BASE_URL), left, centerY - 8, 0xA0A0A0);
        context.drawTextWithShadow(this.textRenderer, Text.translatable("label.minecraft_speedrun_timer.personal_mod_token"), left, centerY + 16, 0xA0A0A0);
        context.drawTextWithShadow(
            this.textRenderer,
            Text.literal(authorizationStatus()),
            left,
            centerY + 54,
            (modClient.isAuthorized() || modClient.isWaitingForMatchStart()) ? 0xFF80FF80 : 0xFFFF8A8A
        );
        context.drawTextWithShadow(
            this.textRenderer,
            Text.literal(message == null ? "" : message),
            left,
            centerY + 126,
            0xFFFFD37F
        );
    }

    private void saveAndAuthorize() {
        if (!persist()) {
            return;
        }
        message = MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.authorizing");
        modClient.authorizeConfiguredToken(result ->
            MinecraftClient.getInstance().execute(() -> message = result.ok() ? MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.token_accepted") : result.message())
        );
    }

    private boolean persist() {
        config.setModToken(modTokenField.getText());
        try {
            config.save();
            return true;
        } catch (IOException exception) {
            message = MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.config_save_failed");
            return false;
        }
    }

    private void toggleTokenVisibility() {
        tokenVisible = !tokenVisible;
        if (tokenVisibilityButton != null) {
            tokenVisibilityButton.setMessage(tokenVisibilityText());
        }
    }

    private Text tokenVisibilityText() {
        return Text.literal(tokenVisible ? "*" : "👁");
    }

    private void renderHiddenTokenOverlay(DrawContext context, int x, int y) {
        if (tokenVisible || modTokenField == null || !modTokenField.isVisible()) {
            return;
        }
        context.fill(x + 2, y + 2, x + 248, y + 18, 0xFF000000);
        String token = modTokenField.getText();
        if (token.isBlank()) {
            return;
        }
        String mask = "*".repeat(Math.min(token.length(), 32));
        context.drawTextWithShadow(this.textRenderer, Text.literal(mask), x + 4, y + 6, 0xFFE0E0E0);
    }

    private String authorizationStatus() {
        if (!modClient.isAuthorized()) {
            String hudStatus = modClient.getHudStatus();
            if (hudStatus != null && !hudStatus.isBlank()) {
                if (modClient.isWaitingForMatchStart()) {
                    return MinecraftSpeedrunModClient.translate("status.minecraft_speedrun_timer.authorized") + " (" + hudStatus + ")";
                }
                return MinecraftSpeedrunModClient.translate("status.minecraft_speedrun_timer.not_authorized") + " (" + hudStatus + ")";
            }
            return MinecraftSpeedrunModClient.translate("status.minecraft_speedrun_timer.not_authorized");
        }
        String login = modClient.getAuthorizedLogin();
        if (login == null || login.isBlank()) {
            return MinecraftSpeedrunModClient.translate("status.minecraft_speedrun_timer.authorized");
        }
        return MinecraftSpeedrunModClient.translate("status.minecraft_speedrun_timer.authorized_as", login);
    }
}
