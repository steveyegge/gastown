package dev.saseq.listeners;

import net.dv8tion.jda.api.entities.Message;
import net.dv8tion.jda.api.entities.User;
import net.dv8tion.jda.api.entities.channel.unions.MessageChannelUnion;
import net.dv8tion.jda.api.events.message.MessageReceivedEvent;
import org.junit.jupiter.api.BeforeEach;
import org.junit.jupiter.api.Test;
import org.junit.jupiter.api.extension.ExtendWith;
import org.mockito.Mock;
import org.mockito.junit.jupiter.MockitoExtension;

import static org.mockito.Mockito.*;

/**
 * Tests for DiscordMessageListener.
 */
@ExtendWith(MockitoExtension.class)
class DiscordMessageListenerTest {

    @Mock
    private MessageReceivedEvent event;

    @Mock
    private User author;

    @Mock
    private Message message;

    @Mock
    private MessageChannelUnion channel;

    private DiscordMessageListener listener;

    @BeforeEach
    void setUp() {
        listener = new DiscordMessageListener();
    }

    @Test
    void onMessageReceived_ignoresBotMessages() {
        // Given a message from a bot
        when(event.getAuthor()).thenReturn(author);
        when(author.isBot()).thenReturn(true);

        // When the listener receives the event
        listener.onMessageReceived(event);

        // Then it should not process the message further
        verify(event, never()).getMessage();
        verify(event, never()).getChannel();
    }

    @Test
    void onMessageReceived_logsHumanMessages() {
        // Given a message from a human user
        when(event.getAuthor()).thenReturn(author);
        when(author.isBot()).thenReturn(false);
        when(author.getName()).thenReturn("TestUser");
        when(event.getMessage()).thenReturn(message);
        when(message.getContentRaw()).thenReturn("Hello, world!");
        when(event.getChannel()).thenReturn(channel);
        when(channel.getName()).thenReturn("general");

        // When the listener receives the event
        listener.onMessageReceived(event);

        // Then it should access the message details for logging
        verify(author).getName();
        verify(channel).getName();
        verify(message).getContentRaw();
    }

    @Test
    void onMessageReceived_handlesSelfMessages() {
        // Given a message from the bot itself (which is also a bot)
        when(event.getAuthor()).thenReturn(author);
        when(author.isBot()).thenReturn(true);

        // When the listener receives the event
        listener.onMessageReceived(event);

        // Then it should ignore the message (bots include self)
        verify(event, never()).getMessage();
    }
}
