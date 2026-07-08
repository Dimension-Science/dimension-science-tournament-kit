package dev.codex.speedrun.timer.mixin;

import dev.codex.speedrun.timer.MinecraftSpeedrunModClient;
import net.minecraft.client.MinecraftClient;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;

@Mixin(MinecraftClient.class)
abstract class MinecraftClientMixin {
    @Inject(method = "handleInputEvents", at = @At("HEAD"), cancellable = true)
    private void minecraftSpeedrunTimer$cancelInputEvents(CallbackInfo ci) {
        MinecraftSpeedrunModClient mod = MinecraftSpeedrunModClient.getInstance();
        if (mod != null && mod.isWaitingForMatchStart()) {
            ci.cancel();
        }
    }

    @Inject(method = "handleBlockBreaking", at = @At("HEAD"), cancellable = true)
    private void minecraftSpeedrunTimer$cancelBlockBreaking(boolean breaking, CallbackInfo ci) {
        MinecraftSpeedrunModClient mod = MinecraftSpeedrunModClient.getInstance();
        if (mod != null && mod.isWaitingForMatchStart()) {
            ci.cancel();
        }
    }
}
