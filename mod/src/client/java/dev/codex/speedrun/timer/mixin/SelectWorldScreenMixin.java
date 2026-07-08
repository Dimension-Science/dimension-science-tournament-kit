package dev.codex.speedrun.timer.mixin;

import dev.codex.speedrun.timer.MinecraftSpeedrunModClient;
import net.minecraft.client.MinecraftClient;
import net.minecraft.client.gui.screen.Screen;
import net.minecraft.client.gui.screen.world.SelectWorldScreen;
import net.minecraft.client.gui.widget.ButtonWidget;
import net.minecraft.text.Text;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;

@Mixin(SelectWorldScreen.class)
abstract class SelectWorldScreenMixin extends Screen {
    protected SelectWorldScreenMixin(Text title) {
        super(title);
    }

    @Inject(method = "init", at = @At("TAIL"))
    private void minecraftSpeedrunTimer$addAuthButton(CallbackInfo ci) {
        MinecraftClient client = MinecraftClient.getInstance();
        MinecraftSpeedrunModClient modClient = MinecraftSpeedrunModClient.getInstance();
        if (client == null || modClient == null) {
            return;
        }

        int buttonWidth = 132;
        int x = this.width - buttonWidth - 12;
        int y = 12;
        this.addDrawableChild(ButtonWidget.builder(tokenButtonText(modClient), button ->
                modClient.openAuthScreen(this, MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.set_token_before_world")))
            .dimensions(x, y, buttonWidth, 20)
            .build());
    }

    private static Text tokenButtonText(MinecraftSpeedrunModClient modClient) {
        return Text.translatable(modClient.tokenButtonTranslationKey());
    }
}
