package dev.saseq.config;

import dev.saseq.listeners.DiscordMessageListener;
import org.junit.jupiter.api.Test;

import static org.junit.jupiter.api.Assertions.*;

/**
 * Tests for DiscordMcpConfig.
 */
class DiscordMcpConfigTest {

    @Test
    void discordMessageListener_createsListenerBean() {
        // Given a config instance
        DiscordMcpConfig config = new DiscordMcpConfig();

        // When we create the listener bean
        DiscordMessageListener listener = config.discordMessageListener();

        // Then it should not be null
        assertNotNull(listener);
    }

    @Test
    void discordMessageListener_returnsNewInstance() {
        // Given a config instance
        DiscordMcpConfig config = new DiscordMcpConfig();

        // When we create listener beans twice
        DiscordMessageListener listener1 = config.discordMessageListener();
        DiscordMessageListener listener2 = config.discordMessageListener();

        // Then they should be different instances (Spring manages singleton scope, but method returns new)
        assertNotSame(listener1, listener2);
    }
}
