#!/usr/bin/env bun
/**
 * Astro Agent — AI-powered development agent for Airflow.
 *
 * Built on Pi (https://github.com/badlogic/pi-mono) without forking.
 * All customization is done via the extension API and resource loader config.
 */

import {
  createAgentSession,
  InteractiveMode,
  DefaultResourceLoader,
} from "@mariozechner/pi-coding-agent";
import { brandingExtension } from "./extensions/branding";
import { airflowExtension } from "./extensions/airflow";
import { gatewayExtension } from "./extensions/gateway";

async function main() {
  const cwd = process.cwd();

  const resourceLoader = new DefaultResourceLoader({
    cwd,
    extensionFactories: [
      brandingExtension,
      airflowExtension,
      gatewayExtension,
    ],
  });
  await resourceLoader.reload();

  const { session, modelFallbackMessage } = await createAgentSession({
    cwd,
    resourceLoader,
  });

  const interactive = new InteractiveMode(session, {
    modelFallbackMessage,
  });
  await interactive.run();
}

main().catch((err) => {
  console.error(err);
  process.exit(1);
});
