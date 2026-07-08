package dev.codex.speedrun.timer.mixin;

import dev.codex.speedrun.timer.MinecraftSpeedrunModClient;
import net.minecraft.client.MinecraftClient;
import net.minecraft.client.gui.DrawContext;
import net.minecraft.client.gui.screen.Screen;
import net.minecraft.client.gui.screen.world.CreateWorldScreen;
import net.minecraft.client.gui.screen.world.WorldCreator;
import net.minecraft.client.gui.widget.ButtonWidget;
import net.minecraft.text.Text;
import org.spongepowered.asm.mixin.Mixin;
import org.spongepowered.asm.mixin.Shadow;
import org.spongepowered.asm.mixin.Unique;
import org.spongepowered.asm.mixin.injection.At;
import org.spongepowered.asm.mixin.injection.Inject;
import org.spongepowered.asm.mixin.injection.callback.CallbackInfo;

@Mixin(CreateWorldScreen.class)
abstract class CreateWorldScreenMixin extends Screen {
    @Unique
    private ButtonWidget minecraftSpeedrunTimer$tokenButton;

    protected CreateWorldScreenMixin(Text title) {
        super(title);
    }

    @Shadow
    public abstract WorldCreator getWorldCreator();

    @Inject(method = "init", at = @At("TAIL"))
    private void minecraftSpeedrunTimer$addTokenButton(CallbackInfo ci) {
        MinecraftClient client = MinecraftClient.getInstance();
        MinecraftSpeedrunModClient modClient = MinecraftSpeedrunModClient.getInstance();
        if (client == null || modClient == null) {
            return;
        }

        if (modClient.isConfigured() && !modClient.isAuthorized() && !modClient.isWaitingForMatchStart()) {
            modClient.authorizeConfiguredTokenAsync();
        }

        int buttonWidth = 130;
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

    @Inject(method = "createLevel", at = @At("HEAD"), cancellable = true)
    private void minecraftSpeedrunTimer$requireTokenBeforeCreate(CallbackInfo ci) {
        MinecraftClient client = MinecraftClient.getInstance();
        MinecraftSpeedrunModClient modClient = MinecraftSpeedrunModClient.getInstance();
        if (client == null || modClient == null) {
            return;
        }

        String seed = getWorldCreator().getSeed();
        String seedError = modClient.validateSeedBeforeWorldCreate(seed);
        if (!seedError.isBlank()) {
            ci.cancel();
            modClient.openMessageScreen(this, seedError);
            return;
        }

        if (!modClient.isConfigured()) {
            ci.cancel();
            modClient.openAuthScreen(this, MinecraftSpeedrunModClient.translate("message.minecraft_speedrun_timer.must_set_token_before_world"));
            return;
        }

        modClient.markFreshWorldCreateApproved();
    }

    private static Text tokenButtonText(MinecraftSpeedrunModClient modClient) {
        return Text.translatable(modClient.tokenButtonTranslationKey());
    }
}
