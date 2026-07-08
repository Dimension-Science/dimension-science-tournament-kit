package dev.codex.speedrun.timer.mixin;

import dev.codex.speedrun.timer.MinecraftSpeedrunModClient;
import net.minecraft.client.gui.DrawContext;
import net.minecraft.client.gui.hud.InGameHud;
import net.minecraft.client.render.RenderTickCounter;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;

@Mixin(InGameHud.class)
abstract class InGameHudMixin {
    @Inject(method = "render", at = @At("HEAD"))
    private void minecraftSpeedrunTimer$render(DrawContext context, RenderTickCounter tickCounter, CallbackInfo ci) {
        MinecraftSpeedrunModClient.getInstance().renderHudOverlay(context);
    }
}
