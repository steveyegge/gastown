package dev.saseq.config;

import dev.saseq.listeners.DiscordMessageListener;
import net.dv8tion.jda.api.JDA;
import net.dv8tion.jda.api.JDABuilder;
import org.springframework.beans.factory.annotation.Value;
import org.springframework.context.annotation.Bean;
import org.springframework.context.annotation.Configuration;

/**
 * Configuration for Discord MCP integration.
 * Initializes JDA and registers event listeners.
 */
@Configuration
public class DiscordMcpConfig {

    @Value("${discord.token}")
    private String discordToken;

    @Bean
    public DiscordMessageListener discordMessageListener() {
        return new DiscordMessageListener();
    }

    @Bean
    public JDA jda(DiscordMessageListener discordMessageListener) throws Exception {
        return JDABuilder.createDefault(discordToken)
                .addEventListeners(discordMessageListener)
                .build()
                .awaitReady();
    }
}
