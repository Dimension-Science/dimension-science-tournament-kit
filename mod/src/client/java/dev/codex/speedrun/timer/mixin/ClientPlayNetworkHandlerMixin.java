package dev.codex.speedrun.timer.mixin;

import dev.codex.speedrun.timer.MinecraftSpeedrunModClient;
import net.minecraft.client.network.ClientPlayNetworkHandler;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;

@Mixin(ClientPlayNetworkHandler.class)
abstract class ClientPlayNetworkHandlerMixin {
    @Inject(method = "sendChatCommand", at = @At("HEAD"), cancellable = true)
    private void minecraftSpeedrunTimer$blockCommandBeforeDragonKill(String command, CallbackInfo ci) {
        MinecraftSpeedrunModClient mod = MinecraftSpeedrunModClient.getInstance();
        if (mod == null || !mod.shouldBlockCommandBeforeDragonKill(command)) {
            return;
        }

        mod.showCommandBlockedMessage();
        ci.cancel();
    }
}
