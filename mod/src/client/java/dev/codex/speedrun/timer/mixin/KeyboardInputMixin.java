package dev.codex.speedrun.timer.mixin;

import dev.codex.speedrun.timer.MinecraftSpeedrunModClient;
import net.minecraft.client.input.Input;
import net.minecraft.client.input.KeyboardInput;
import net.minecraft.util.PlayerInput;
import net.minecraft.util.math.Vec2f;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;

@Mixin(KeyboardInput.class)
abstract class KeyboardInputMixin extends Input {
    @Inject(method = "tick", at = @At("TAIL"))
    private void minecraftSpeedrunTimer$lockMovement(CallbackInfo ci) {
        MinecraftSpeedrunModClient mod = MinecraftSpeedrunModClient.getInstance();
        if (mod != null && (mod.isWaitingForMatchStart() || mod.isMatchLost())) {
            this.playerInput = new PlayerInput(false, false, false, false, false, false, false);
            this.movementVector = Vec2f.ZERO;
        }
    }
}
