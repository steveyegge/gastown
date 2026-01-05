/**
 * Gas Town GUI - State Management
 *
 * Simple reactive state store with subscription support.
 * No external dependencies - just plain JavaScript.
 */

// State store
const store = {
  status: null,
  convoys: [],
  agents: [],
  events: [],
  mail: [],
};

// Subscribers by key
const subscribers = new Map();

// Maximum events to keep
const MAX_EVENTS = 500;

// Subscribe to state changes
export function subscribe(key, callback) {
  if (!subscribers.has(key)) {
    subscribers.set(key, new Set());
  }
  subscribers.get(key).add(callback);

  // Return unsubscribe function
  return () => {
    subscribers.get(key).delete(callback);
  };
}

// Notify subscribers of changes
function notify(key) {
  const callbacks = subscribers.get(key);
  if (callbacks) {
    callbacks.forEach(cb => cb(store[key]));
  }
}

// State mutations
export const state = {
  // Get current state
  get(key) {
    return store[key];
  },

  // Set status
  setStatus(status) {
    store.status = status;
    notify('status');

    // Extract agents from status if present
    if (status?.agents) {
      this.setAgents(status.agents);
    }
  },

  // Set convoys
  setConvoys(convoys) {
    store.convoys = convoys || [];
    notify('convoys');
  },

  // Update single convoy
  updateConvoy(convoy) {
    if (!convoy?.id) return;

    const index = store.convoys.findIndex(c => c.id === convoy.id);
    if (index >= 0) {
      store.convoys[index] = { ...store.convoys[index], ...convoy };
    } else {
      store.convoys.unshift(convoy);
    }
    notify('convoys');
  },

  // Set agents
  setAgents(agents) {
    store.agents = agents || [];
    notify('agents');
  },

  // Get agents
  getAgents() {
    return store.agents || [];
  },

  // Get rigs from status
  getRigs() {
    return store.status?.rigs || [];
  },

  // Add event
  addEvent(event) {
    // Add timestamp if missing
    if (!event.timestamp) {
      event.timestamp = new Date().toISOString();
    }

    // Add to beginning
    store.events.unshift(event);

    // Trim to max
    if (store.events.length > MAX_EVENTS) {
      store.events = store.events.slice(0, MAX_EVENTS);
    }

    notify('events');
  },

  // Clear events
  clearEvents() {
    store.events = [];
    notify('events');
  },

  // Set mail
  setMail(mail) {
    store.mail = mail || [];
    notify('mail');
  },

  // Mark mail as read
  markMailRead(id) {
    const mail = store.mail.find(m => m.id === id);
    if (mail) {
      mail.read = true;
      notify('mail');
    }
  },
};

// Export store for debugging
export { store };
