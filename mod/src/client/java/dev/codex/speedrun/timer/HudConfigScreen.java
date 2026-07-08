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

public final class HudConfigScreen extends Screen {
    private static final Identifier LOGO_TEXTURE = Identifier.of("minecraft_speedrun_timer", "textures/gui/logo.png");

    private final Screen parent;
    private final ModConfig config;
    private final MinecraftSpeedrunModClient modClient;
    private TextFieldWidget hudXField;
    private TextFieldWidget hudYField;
    private ButtonWidget coordinatesButton;
    private TextFieldWidget coordinatesYField;
    private String message;

    public HudConfigScreen(Screen parent, ModConfig config, MinecraftSpeedrunModClient modClient) {
        super(Text.translatable("screen.minecraft_speedrun_timer.settings"));
        this.parent = parent;
        this.config = config;
        this.modClient = modClient;
        this.message = modClient.getHudStatus();
    }

    @Override
    protected void init() {
        int left = this.width / 2 - 150;
        int top = this.height / 2 - 90;

        hudXField = createField(left, top + 28, 145, "HUD X", Integer.toString(modClient.resolveHudX(this.width)), 6);
        hudYField = createField(left + 155, top + 28, 145, "HUD Y", Integer.toString(modClient.resolveHudY(this.height)), 6);
        
        coordinatesButton = addDrawableChild(ButtonWidget.builder(coordinatesText(), button -> toggleCoordinates())
            .dimensions(left, top + 76, 145, 20)
            .build());
        coordinatesYField = createField(left + 155, top + 76, 145, "Coords Y", Integer.toString(config.getCoordinatesOffsetY()), 4);

        addDrawableChild(ButtonWidget.builder(Text.translatable("button.minecraft_speedrun_timer.save"), button -> saveOnly())
            .dimensions(left, top + 116, 145, 20)
            .build());
        addDrawableChild(ButtonWidget.builder(Text.translatable("button.minecraft_speedrun_timer.reset_hud"), button -> resetHudPosition())
            .dimensions(left + 155, top + 116, 145, 20)
            .build());
        addDrawableChild(ButtonWidget.builder(Text.translatable("button.minecraft_speedrun_timer.close"), button -> close())
            .dimensions(left, top + 144, 300, 20)
            .build());

        setInitialFocus(hudXField);
    }

    private TextFieldWidget createField(int x, int y, int width, String placeholder, String value, int maxLength) {
        TextFieldWidget field = new TextFieldWidget(this.textRenderer, x, y, width, 20, Text.literal(placeholder));
        field.setMaxLength(maxLength);
        field.setText(value);
        addDrawableChild(field);
        return field;
    }

    @Override
    public void close() {
        MinecraftClient.getInstance().setScreen(parent);
    }

    @Override
    public void render(DrawContext context, int mouseX, int mouseY, float delta) {
        context.fill(0, 0, this.width, this.height, 0xCC101018);
        super.render(context, mouseX, mouseY, delta);

        int left = this.width / 2 - 150;
        int top = this.height / 2 - 90;
        int muted = 0xFF808080;
        int label = 0xFFA0A0A0;
        int white = 0xFFFFFFFF;
        int accent = 0xFFFFD37F;

        context.drawTexture(RenderPipelines.GUI_TEXTURED, LOGO_TEXTURE, this.width / 2 - 16, top - 58, 0.0F, 0.0F, 32, 32, 64, 64, 64, 64);
        context.drawCenteredTextWithShadow(this.textRenderer, this.title, this.width / 2, top - 20, white);
        context.drawTextWithShadow(this.textRenderer, Text.translatable("label.minecraft_speedrun_timer.hud_x"), left, top + 18, label);
        context.drawTextWithShadow(this.textRenderer, Text.translatable("label.minecraft_speedrun_timer.hud_y"), left + 155, top + 18, label);
        context.drawTextWithShadow(this.textRenderer, Text.translatable("label.minecraft_speedrun_timer.coordinates_y"), left + 155, top + 66, label);
        context.drawTextWithShadow(this.textRenderer, Text.translatable("label.minecraft_speedrun_timer.coordinates_hint"), left, top + 102, muted);
        context.drawTextWithShadow(this.textRenderer, Text.translatable("label.minecraft_speedrun_timer.preview"), left, top + 176, label);
        drawPreviewCoordinates(context, left, top + 188);
        context.drawTextWithShadow(this.textRenderer, Text.literal(message == null ? "" : message), left, top + 210, accent);
        modClient.renderHudPositionPreview(context, parseInt(hudXField.getText(), config.getHudX()), parseInt(hudYField.getText(), config.getHudY()));
    }

    private void saveOnly() {
        if (persist()) {
            message = MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.settings_saved");
        }
    }

    private void resetHudPosition() {
        hudXField.setText("10");
        hudYField.setText("10");
        coordinatesYField.setText("59");
        saveOnly();
    }

    private void toggleCoordinates() {
        config.setCoordinatesEnabled(!config.isCoordinatesEnabled());
        coordinatesButton.setMessage(coordinatesText());
        saveOnly();
    }

    private Text coordinatesText() {
        return Text.translatable(
            "button.minecraft_speedrun_timer.coordinates_above_hotbar",
            MinecraftSpeedrunModClient.translate(config.isCoordinatesEnabled() ? "state.minecraft_speedrun_timer.on" : "state.minecraft_speedrun_timer.off")
        );
    }

    private boolean persist() {
        config.setHudX(parseInt(hudXField.getText(), config.getHudX()));
        config.setHudY(parseInt(hudYField.getText(), config.getHudY()));
        config.setCoordinatesOffsetY(parseInt(coordinatesYField.getText(), config.getCoordinatesOffsetY()));
        config.setHudReferenceSize(this.width, this.height);

        try {
            config.save();
            modClient.reloadConfig();
            return true;
        } catch (IOException exception) {
            message = MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.config_save_failed");
            return false;
        }
    }

    private static int parseInt(String value, int fallback) {
        try {
            return Integer.parseInt(value.trim());
        } catch (Exception exception) {
            return fallback;
        }
    }

    private void drawPreviewCoordinates(DrawContext context, int x, int y) {
        x = drawPreviewPart(context, "X ", x, y, 0xFFFFD84D);
        x = drawPreviewPart(context, "123", x, y, 0xFFFFFFFF);
        x = drawPreviewPart(context, " Y ", x, y, 0xFFFFD84D);
        x = drawPreviewPart(context, "64", x, y, 0xFFFFFFFF);
        x = drawPreviewPart(context, " Z ", x, y, 0xFFFFD84D);
        drawPreviewPart(context, "-456", x, y, 0xFFFFFFFF);
    }

    private int drawPreviewPart(DrawContext context, String text, int x, int y, int color) {
        context.drawTextWithShadow(this.textRenderer, Text.literal(text), x, y, color);
        return x + this.textRenderer.getWidth(text);
    }
}
