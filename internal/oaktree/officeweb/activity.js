export function advanceBubble(actor, now) {
  if (actor.bubble && actor.bubbleUntil <= now) actor.bubble = null;
  if (!actor.bubble && actor.bubbleQueue.length) {
    actor.bubble = actor.bubbleQueue.shift();
    actor.bubbleUntil = now + 10000;
  }
  return actor.bubble;
}

export function createActivityTracker() {
  let initialized = false;
  const seen = new Set();
  return {
    observe(entries) {
      const fresh = [];
      for (const entry of entries) {
        if (seen.has(entry.key)) continue;
        seen.add(entry.key);
        if (initialized) fresh.push(entry);
      }
      initialized = true;
      while (seen.size > 2000) seen.delete(seen.values().next().value);
      return fresh;
    },
  };
}
