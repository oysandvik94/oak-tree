package oaktree

import (
	"context"
	"fmt"
	osexec "os/exec"
	"path/filepath"
)

// PiExtensionSource is kept in the binary so a Pi session never depends on a
// mutable global Pi configuration or a checked-out oak-tree source tree.
const PiExtensionSource = `import { Type } from "typebox";

export default function (pi) {
  async function hook(event, extra = {}) {
    const oak = process.env.OAK_TREE_SESSION_ID;
    if (!oak) return;
    const command = process.env.OAK_TREE_HOOK || "oak-tree";
    const args = ["hook", "agent-event", "--oak-session", oak, "--event", event];
    for (const [key, value] of Object.entries(extra)) {
      if (value === undefined || value === null || value === "") continue;
      if (!["cwd", "session_id", "session_file", "todo_total", "todo_pending", "todo_in_progress", "todo_completed", "todo_json"].includes(key)) continue;
      args.push("--" + key.replaceAll("_", "-"), String(value));
    }
    for (let attempt = 0; attempt < 20; attempt++) {
      try {
        const result = await pi.exec(command, args, { timeout: 2000 });
        if (result.code === 0) return;
      } catch (_) {}
      await new Promise((resolve) => setTimeout(resolve, 50));
    }
  }
  const identity = (ctx) => ({
    cwd: ctx.cwd,
    session_id: ctx.sessionManager.getSessionId(),
    session_file: ctx.sessionManager.getSessionFile(),
  });
  let currentIdentity;
  const todoSummary = (details) => {
    if (!Array.isArray(details?.tasks)) return;
    const summary = { todo_total: 0, todo_pending: 0, todo_in_progress: 0, todo_completed: 0, todo_json: "[]" };
    const tasks = [];
    for (const task of details.tasks) {
      if (!["pending", "in_progress", "completed"].includes(task?.status) || typeof task?.subject !== "string" || !task.subject.trim()) continue;
      const key = "todo_" + task.status;
      summary[key]++;
      summary.todo_total++;
      tasks.push({ subject: task.subject.trim(), status: task.status });
    }
    summary.todo_json = JSON.stringify(tasks);
    return summary;
  };
  const reportTodos = async (ctx, details) => {
    const summary = todoSummary(details);
    if (summary) await hook("todo", { ...identity(ctx), ...summary });
  };
  pi.on("session_start", async (_event, ctx) => {
    currentIdentity = identity(ctx);
    await hook("session_start", currentIdentity);
    let latest = { tasks: [] };
    for (const entry of ctx.sessionManager.getBranch()) {
      const message = entry?.type === "message" ? entry.message : undefined;
      if (message?.role === "toolResult" && message.toolName === "todo" && Array.isArray(message.details?.tasks)) latest = message.details;
    }
    await reportTodos(ctx, latest);
    if (!pi.getAllTools().some((tool) => tool.name === "ask_user_question")) registerQuestionTool();
  });
  pi.on("agent_start", async (_event, ctx) => { await hook("agent_start", identity(ctx)); });
  pi.on("agent_settled", async (_event, ctx) => { await hook("agent_settled", identity(ctx)); });
  pi.on("session_shutdown", async (_event, ctx) => {
    await hook("session_shutdown", identity(ctx));
    currentIdentity = undefined;
  });

  let externalQuestion;
  let externalQuestionHook;
  pi.on("tool_execution_start", (event, ctx) => {
    if (event.toolName === "ask_user_question") externalQuestion = identity(ctx);
  });
  pi.events.on("rpiv:ask-user:prompt", () => {
    if (externalQuestion) externalQuestionHook = hook("question", externalQuestion);
  });
  pi.on("tool_execution_end", async (event, ctx) => {
    if (event.toolName === "todo" && !event.isError) await reportTodos(ctx, event.result?.details);
    if (event.toolName !== "ask_user_question") return;
    const questionHook = externalQuestionHook;
    externalQuestion = undefined;
    externalQuestionHook = undefined;
    if (questionHook) {
      await questionHook;
      await hook("question_answered", identity(ctx));
    }
  });

  function registerQuestionTool() {
    pi.registerTool({
      name: "question",
      label: "Ask user",
      description: "Ask the user a blocking question. MUST use this whenever progress requires user choice/input; do not ask in ordinary prose.",
      promptSnippet: "Ask the user a blocking question",
      promptGuidelines: ["Use the question tool whenever progress needs user input; never ask blocking questions only in prose."],
      executionMode: "sequential",
      parameters: Type.Object({
        question: Type.String(),
        options: Type.Optional(Type.Array(Type.String())),
      }),
      async execute(_toolCallId, params, _signal, _onUpdate, ctx) {
        await hook("question", identity(ctx));
        let answer;
        if (params.options && params.options.length) {
          answer = await ctx.ui.select(params.question, params.options);
        } else {
          answer = await ctx.ui.input(params.question);
        }
        await hook("question_answered", identity(ctx));
        return { content: [{ type: "text", text: answer || "(cancelled)" }], details: { answer } };
      },
    });
  }
}
`

func PiExtensionPath(paths Paths) string {
	dir := paths.PiDir
	if dir == "" {
		dir = filepath.Join(paths.StateDir, "pi")
	}
	return filepath.Join(dir, "oak-tree-extension.ts")
}

func EnsurePiExtension(paths Paths) (string, error) {
	path := PiExtensionPath(paths)
	if err := atomicWrite(path, []byte(PiExtensionSource), 0o600); err != nil {
		return "", err
	}
	return path, nil
}

func PiCommand(ctx context.Context, paths Paths, sessionID string) ([]string, error) {
	if _, err := osexec.LookPath("pi"); err != nil {
		return nil, fmt.Errorf("Pi CLI is not installed or not on PATH: %w", err)
	}
	extension, err := EnsurePiExtension(paths)
	if err != nil {
		return nil, fmt.Errorf("prepare Pi extension: %w", err)
	}
	hook, err := hookExecutable()
	if err != nil {
		return nil, fmt.Errorf("prepare Pi hook executable: %w", err)
	}
	return []string{"env", "OAK_TREE_SESSION_ID=" + sessionID, "OAK_TREE_HOOK=" + hook, "pi", "-e", extension}, nil
}
