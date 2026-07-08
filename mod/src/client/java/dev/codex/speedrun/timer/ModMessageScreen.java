package dev.codex.speedrun.timer;

import net.minecraft.client.MinecraftClient;
import net.minecraft.client.gui.DrawContext;
import net.minecraft.client.gui.screen.Screen;
import net.minecraft.client.gui.widget.ButtonWidget;
import net.minecraft.text.Text;

import java.util.ArrayList;
import java.util.List;

final class ModMessageScreen extends Screen {
    private final Screen parent;
    private final String message;

    ModMessageScreen(Screen parent, String message) {
        super(Text.translatable("screen.minecraft_speedrun_timer.notice"));
        this.parent = parent;
        this.message = message == null ? "" : message;
    }

    @Override
    protected void init() {
        int buttonWidth = 220;
        int centerX = this.width / 2;
        int centerY = this.height / 2;
        addDrawableChild(ButtonWidget.builder(Text.translatable("button.minecraft_speedrun_timer.close"), button -> close())
            .dimensions(centerX - buttonWidth / 2, centerY + 38, buttonWidth, 20)
            .build());
    }

    @Override
    public void close() {
        MinecraftClient.getInstance().setScreen(parent);
    }

    @Override
    public void render(DrawContext context, int mouseX, int mouseY, float delta) {
        super.render(context, mouseX, mouseY, delta);
        int centerX = this.width / 2;
        int y = this.height / 2 - 46;
        context.drawCenteredTextWithShadow(this.textRenderer, this.title, centerX, y, 0xFFFFFF);

        int maxWidth = Math.min(340, this.width - 40);
        List<String> lines = wrapMessage(message, maxWidth);
        int lineY = y + 28;
        for (String line : lines) {
            context.drawCenteredTextWithShadow(this.textRenderer, Text.literal(line), centerX, lineY, 0xFFFFD37F);
            lineY += 12;
        }
    }

    private List<String> wrapMessage(String value, int maxWidth) {
        List<String> lines = new ArrayList<>();
        String[] words = value.split("\\s+");
        StringBuilder current = new StringBuilder();
        for (String word : words) {
            if (word.isBlank()) {
                continue;
            }
            String candidate = current.isEmpty() ? word : current + " " + word;
            if (!current.isEmpty() && this.textRenderer.getWidth(candidate) > maxWidth) {
                lines.add(current.toString());
                current.setLength(0);
                current.append(word);
            } else {
                current.setLength(0);
                current.append(candidate);
            }
        }
        if (!current.isEmpty()) {
            lines.add(current.toString());
        }
        if (lines.isEmpty()) {
            lines.add("");
        }
        return lines;
    }
}
