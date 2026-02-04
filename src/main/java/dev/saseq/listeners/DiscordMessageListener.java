package dev.saseq.listeners;

import net.dv8tion.jda.api.events.message.MessageReceivedEvent;
import net.dv8tion.jda.api.hooks.ListenerAdapter;
import org.slf4j.Logger;
import org.slf4j.LoggerFactory;

/**
 * Listens for Discord messages and logs them.
 * Ignores messages from bots (including self).
 */
public class DiscordMessageListener extends ListenerAdapter {

    private static final Logger logger = LoggerFactory.getLogger(DiscordMessageListener.class);

    @Override
    public void onMessageReceived(MessageReceivedEvent event) {
        // Ignore messages from bots (including self)
        if (event.getAuthor().isBot()) {
            return;
        }

        String author = event.getAuthor().getName();
        String channel = event.getChannel().getName();
        String content = event.getMessage().getContentRaw();

        logger.info("Message received - Author: {}, Channel: {}, Content: {}", author, channel, content);
    }
}
