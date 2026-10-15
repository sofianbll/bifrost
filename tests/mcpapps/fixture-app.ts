import { App } from "@modelcontextprotocol/ext-apps/app-with-deps";
import type { CallToolResult } from "@modelcontextprotocol/client";

const appRoot = document.querySelector<HTMLElement>("[data-testid='mcp-app-fixture']");
const status = document.querySelector<HTMLElement>("[data-testid='fixture-status']");
const inputView = document.querySelector<HTMLElement>("[data-testid='tool-input']");
const resultView = document.querySelector<HTMLElement>("[data-testid='tool-result']");
const callbackView = document.querySelector<HTMLElement>("[data-testid='callback-result']");
const stateInput = document.querySelector<HTMLInputElement>("[data-testid='state-input']");

if (!appRoot || !status || !inputView || !resultView || !callbackView || !stateInput) {
  throw new Error("MCP Apps fixture view is missing a required element");
}
const view = appRoot;
const statusText = status;
const inputElement = inputView;
const resultElement = resultView;
const callbackElement = callbackView;
const stateElement = stateInput;

const originalState = stateElement.value;
const saveStateTool = view.dataset.saveTool;
const readStateTool = view.dataset.readTool;
if (!saveStateTool || !readStateTool) {
  throw new Error("MCP Apps fixture is missing its source-qualified callback names");
}
const app = new App({ name: "Bifrost MCP Apps fixture", version: "1.0.0" });

function show(element: HTMLElement, value: unknown): void {
  element.textContent = JSON.stringify(value, null, 2);
}

app.ontoolinput = (params) => {
  show(inputElement, params.arguments);
  statusText.textContent = "Host input received";
  view.dataset.status = "input-received";
};

app.ontoolresult = (result: CallToolResult) => {
  show(resultElement, {
    content: result.content,
    structuredContent: result.structuredContent,
    meta: result._meta,
    isError: result.isError,
  });
  statusText.textContent = result.isError ? "Host result is error" : "Host result received";
  view.dataset.status = "result-received";
};

app.onerror = (error) => {
  statusText.textContent = `App error: ${error.message}`;
  view.dataset.status = "error";
};

async function callCallback(name: string, args: Record<string, unknown> = {}): Promise<void> {
  callbackElement.textContent = `Calling ${name}`;
  try {
    const result = await app.callServerTool({ name, arguments: args });
    show(callbackElement, {
      name,
      content: result.content,
      structuredContent: result.structuredContent,
      meta: result._meta,
      isError: result.isError,
    });
  } catch (error) {
    show(callbackElement, { name, error: error instanceof Error ? error.message : String(error) });
  }
}

document.querySelector("[data-testid='save-state']")?.addEventListener("click", () =>
  callCallback(saveStateTool, { state: stateElement.value }),
);
document.querySelector("[data-testid='read-state']")?.addEventListener("click", () =>
  callCallback(readStateTool),
);
document.querySelector("[data-testid='change-state']")?.addEventListener("click", () => {
  stateElement.value = "changed-state";
  return callCallback(saveStateTool, { state: stateElement.value });
});
document.querySelector("[data-testid='restore-state']")?.addEventListener("click", () => {
  stateElement.value = originalState;
  return callCallback(saveStateTool, { state: stateElement.value });
});
document.querySelector("[data-testid='corrupt-state']")?.addEventListener("click", () => {
  stateElement.value = "corrupt-state";
  return callCallback(saveStateTool, { state: stateElement.value });
});

// The SDK registers the message transport before connect() sends ui/initialize.
// Register the one-shot input/result handlers above before this handshake.
app.connect().then(() => {
  statusText.textContent = "App handshake complete";
  view.dataset.status = "ready";
}).catch((error: unknown) => {
  statusText.textContent = `App initialization failed: ${error instanceof Error ? error.message : String(error)}`;
  view.dataset.status = "error";
});
