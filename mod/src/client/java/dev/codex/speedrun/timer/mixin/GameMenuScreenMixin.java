package dev.codex.speedrun.timer.mixin;

import dev.codex.speedrun.timer.HudConfigScreen;
import dev.codex.speedrun.timer.MinecraftSpeedrunModClient;
import net.minecraft.client.MinecraftClient;
import net.minecraft.client.gui.screen.GameMenuScreen;
import net.minecraft.client.gui.screen.Screen;
import net.minecraft.client.gui.widget.ButtonWidget;
import net.minecraft.text.Text;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;

@Mixin(GameMenuScreen.class)
abstract class GameMenuScreenMixin extends Screen {
    protected GameMenuScreenMixin(Text title) {
        super(title);
    }

    @Inject(method = "init", at = @At("TAIL"))
    private void minecraftSpeedrunTimer$addButton(CallbackInfo ci) {
        MinecraftClient client = MinecraftClient.getInstance();
        if (client == null) {
            return;
        }

        int buttonWidth = 124;
        this.addDrawableChild(ButtonWidget.builder(Text.literal("Timer Position"), button ->
                client.setScreen(new HudConfigScreen(this, MinecraftSpeedrunModClient.getInstance().getConfig(), MinecraftSpeedrunModClient.getInstance())))
            .dimensions(this.width - buttonWidth - 8, 8, buttonWidth, 20)
            .build());
    }
}
