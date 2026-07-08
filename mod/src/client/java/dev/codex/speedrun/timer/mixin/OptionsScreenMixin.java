package dev.codex.speedrun.timer.mixin;

import dev.codex.speedrun.timer.HudConfigScreen;
import dev.codex.speedrun.timer.MinecraftSpeedrunModClient;
import dev.codex.speedrun.timer.TournamentSettingsButton;
import net.minecraft.client.MinecraftClient;
import net.minecraft.client.gui.screen.Screen;
import net.minecraft.client.gui.screen.option.OptionsScreen;
import net.minecraft.text.Text;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;

@Mixin(OptionsScreen.class)
abstract class OptionsScreenMixin extends Screen {

    protected OptionsScreenMixin(Text title) {
        super(title);
    }

    @Inject(method = "init", at = @At("TAIL"))
    private void minecraftSpeedrunTimer$addTournamentSettingsButton(CallbackInfo ci) {
        MinecraftClient client = MinecraftClient.getInstance();
        MinecraftSpeedrunModClient mod = MinecraftSpeedrunModClient.getInstance();
        if (client == null || mod == null) {
            return;
        }

        int x = this.width / 2 - 183;
        int y = this.height / 6 + 72;
        this.addDrawableChild(new TournamentSettingsButton(x, y, 20, 20, Text.literal(""), button ->
                client.setScreen(new HudConfigScreen(this, mod.getConfig(), mod))));
    }
}
