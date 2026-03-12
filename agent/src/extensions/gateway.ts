/**
 * Gateway extension — registers a custom inference gateway provider.
 *
 * Set ASTRO_GATEWAY_URL and ASTRO_GATEWAY_API_KEY to point at your gateway.
 * If not set, falls back to Pi's default provider discovery.
 */

import type { ExtensionFactory } from "@mariozechner/pi-coding-agent";

export const gatewayExtension: ExtensionFactory = (pi) => {
  const gatewayUrl = process.env.ASTRO_GATEWAY_URL;
  const gatewayKey = process.env.ASTRO_GATEWAY_API_KEY;

  if (!gatewayUrl) {
    return;
  }

  pi.registerProvider("astro", {
    baseUrl: gatewayUrl,
    api: "openai-completions",
    apiKey: gatewayKey ?? "",
    authHeader: true,
    headers: {
      "X-Astro-Agent": "true",
    },
    models: [
      {
        id: "claude-sonnet-4-20250514",
        name: "Claude Sonnet 4",
        reasoning: true,
        input: ["text"],
        contextWindow: 200000,
        maxTokens: 64000,
        cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      },
      {
        id: "claude-opus-4-20250514",
        name: "Claude Opus 4",
        reasoning: true,
        input: ["text"],
        contextWindow: 200000,
        maxTokens: 32000,
        cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      },
      {
        id: "gpt-4o",
        name: "GPT-4o",
        reasoning: false,
        input: ["text", "image"],
        contextWindow: 128000,
        maxTokens: 16384,
        cost: { input: 0, output: 0, cacheRead: 0, cacheWrite: 0 },
      },
    ],
  });
};
