package dev.codex.speedrun.timer.mixin;

import dev.codex.speedrun.timer.MinecraftSpeedrunModClient;
import net.minecraft.client.MinecraftClient;
import net.minecraft.client.gui.DrawContext;
import net.minecraft.client.gui.screen.Screen;
import net.minecraft.client.gui.screen.TitleScreen;
import net.minecraft.client.gui.widget.ButtonWidget;
import net.minecraft.text.Text;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.Unique;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;

@Mixin(TitleScreen.class)
abstract class TitleScreenMixin extends Screen {
    @Unique
    private ButtonWidget minecraftSpeedrunTimer$tokenButton;

    protected TitleScreenMixin(Text title) {
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
        this.minecraftSpeedrunTimer$tokenButton = this.addDrawableChild(ButtonWidget.builder(tokenButtonText(modClient), button ->
                modClient.openAuthScreen(this, MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.set_token_before_world")))
            .dimensions(x, y, buttonWidth, 20)
            .build());
    }

    @Inject(method = "render", at = @At("HEAD"))
    private void minecraftSpeedrunTimer$refreshAuthButton(DrawContext context, int mouseX, int mouseY, float delta, CallbackInfo ci) {
        MinecraftSpeedrunModClient modClient = MinecraftSpeedrunModClient.getInstance();
        if (modClient != null && minecraftSpeedrunTimer$tokenButton != null) {
            minecraftSpeedrunTimer$tokenButton.setMessage(tokenButtonText(modClient));
        }
    }

    @Inject(method = "render", at = @At("TAIL"))
    private void minecraftSpeedrunTimer$drawTitleText(DrawContext context, int mouseX, int mouseY, float delta, CallbackInfo ci) {
        context.drawTextWithShadow(this.textRenderer, "Tournament", 2, this.height - 22, 0xFFFFD37F);
    }

    private static Text tokenButtonText(MinecraftSpeedrunModClient modClient) {
        return Text.translatable(modClient.tokenButtonTranslationKey());
    }
}
