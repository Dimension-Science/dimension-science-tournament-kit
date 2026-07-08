package dev.codex.speedrun.timer;

import net.minecraft.client.gui.DrawContext;
import net.minecraft.client.gui.widget.ButtonWidget;
import net.minecraft.util.Identifier;
import net.minecraft.client.gl.RenderPipelines;

public class TournamentSettingsButton extends ButtonWidget {
    private static final Identifier LOGO_TEXTURE = Identifier.of("minecraft_speedrun_timer", "textures/gui/logo.png");

    private static final Identifier BUTTON_TEXTURE = Identifier.of("minecraft", "widget/button");
    private static final Identifier BUTTON_HIGHLIGHTED_TEXTURE = Identifier.of("minecraft", "widget/button_highlighted");
    private static final Identifier BUTTON_DISABLED_TEXTURE = Identifier.of("minecraft", "widget/button_disabled");

    public TournamentSettingsButton(int x, int y, int width, int height, net.minecraft.text.Text message, PressAction onPress) {
        super(x, y, width, height, message, onPress, DEFAULT_NARRATION_SUPPLIER);
    }

    @Override
    protected void drawIcon(DrawContext context, int mouseX, int mouseY, float delta) {
        Identifier backgroundTexture = this.active ? 
            (this.isSelected() || this.isHovered() ? BUTTON_HIGHLIGHTED_TEXTURE : BUTTON_TEXTURE) : 
            BUTTON_DISABLED_TEXTURE;
            
        context.drawGuiTexture(RenderPipelines.GUI, backgroundTexture, this.getX(), this.getY(), this.getWidth(), this.getHeight());

        int iconSize = 14;
        int iconX = this.getX() + (this.getWidth() - iconSize) / 2;
        int iconY = this.getY() + (this.getHeight() - iconSize) / 2;
        context.drawTexture(RenderPipelines.GUI_TEXTURED, LOGO_TEXTURE, iconX, iconY, 0.0F, 0.0F, iconSize, iconSize, 64, 64, 64, 64);
    }
}
