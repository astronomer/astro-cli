/**
 * Branding extension — customizes the TUI appearance for Astro Agent.
 * Replaces header, title, and status text.
 */

import type { ExtensionFactory } from "@mariozechner/pi-coding-agent";

export const brandingExtension: ExtensionFactory = (pi) => {
  pi.on("session_start", (_event, ctx) => {
    ctx.ui.setTitle("Astro Agent");
    ctx.ui.setStatus("brand", "Astro Agent");
  });

  pi.registerCommand("astro-version", {
    description: "Show Astro Agent version",
    handler: async (_args, ctx) => {
      ctx.actions.sendMessage(
        `Astro Agent v${process.env.ASTRO_AGENT_VERSION ?? "dev"}`
      );
    },
  });
};
