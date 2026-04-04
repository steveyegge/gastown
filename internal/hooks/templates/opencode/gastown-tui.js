// Gas Town OpenCode TUI plugin: closes sidebar on session creation.
// Separate from gastown.js because OpenCode requires server and TUI plugins
// to be in different modules (PluginModule vs TuiPluginModule).
export const tui = async (api) => {
  api.event.on("session.created", () => {
    // OpenCode opens the sidebar when navigating to a session.
    // Close it after a short delay to let the TUI render first.
    setTimeout(() => {
      api.command.trigger("sidebar.toggle");
    }, 300);
  });
};
