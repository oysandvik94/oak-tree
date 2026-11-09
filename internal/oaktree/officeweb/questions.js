const element = (tag, className, text) => {
  const node = document.createElement(tag);
  if (className) node.className = className;
  if (text !== undefined) node.textContent = text;
  return node;
};

export function createQuestionPanel({ getToken, refresh }) {
  const launcher = document.getElementById("question-launcher");
  const dialog = document.getElementById("question-dialog");
  const form = document.getElementById("question-form");
  const fields = document.getElementById("question-fields");
  const feedback = document.getElementById("question-feedback");
  const submit = document.getElementById("question-submit");
  const cancel = document.getElementById("question-cancel");
  const drafts = new Map();
  const uncertain = new Set();
  let agents = [];
  let online = false;
  let selectedAgent;
  let active;
  let phase = "open";
  let controls = [];
  let launcherSignature = "";

  const key = (agentId, flowId) => `${agentId}/${flowId}`;
  const pending = () => agents.find((agent) => agent.id === active?.agentId)?.questions?.find((flow) => flow.id === active?.flow.id);
  function message(text, error = false) {
    feedback.textContent = text;
    feedback.classList.toggle("error", error);
  }
  function setDisabled() {
    const disabled = !online || phase !== "open";
    fields.disabled = disabled;
    submit.disabled = disabled;
    cancel.disabled = disabled;
    submit.textContent = phase === "sending" ? "Sending…" : "Send answer";
    if (!online && phase === "open") message("Disconnected. Your draft is kept here; reconnect before sending.", true);
  }
  function renderLauncher() {
    const signature = JSON.stringify([selectedAgent?.id, selectedAgent?.status, selectedAgent?.questions?.map((flow) => flow.id), online]);
    if (signature === launcherSignature) return;
    launcherSignature = signature;
    launcher.replaceChildren();
    launcher.hidden = !selectedAgent || !(selectedAgent.questions?.length || selectedAgent.status === "question");
    if (launcher.hidden) return;
    const flows = selectedAgent.questions || [];
    if (!flows.length) {
      launcher.append(element("p", "inspector-help", "Answer in Pi. Browser answers require a live pi-ask form and the updated oak-tree extension; reload Pi when it is idle."));
      return;
    }
    const button = element("button", "answer-button", flows.length === 1 ? "Answer question" : `Answer questions · ${flows.length}`);
    button.type = "button";
    button.disabled = !online;
    button.addEventListener("click", () => open(selectedAgent));
    launcher.append(button);
  }
  function collect() {
    const answers = Object.create(null);
    for (const control of controls) {
      const values = control.options.filter((input) => input.checked).map((input) => input.value);
      const answer = {};
      if (values.length) answer.values = values;
      if (control.custom.value.trim()) answer.customText = control.custom.value.trim();
      if (control.note.value.trim()) answer.note = control.note.value.trim();
      if (Object.keys(answer).length) answers[control.id] = answer;
    }
    return answers;
  }
  function saveDraft() {
    if (active && phase === "open") {
      drafts.set(key(active.agentId, active.flow.id), collect());
      while (drafts.size > 32) drafts.delete(drafts.keys().next().value);
    }
  }
  function textArea(labelText, placeholder, value = "") {
    const label = element("label", "answer-text-field");
    label.append(element("span", "", labelText));
    const input = element("textarea");
    input.rows = 2;
    input.maxLength = 8192;
    input.placeholder = placeholder;
    input.value = value;
    label.append(input);
    return { label, input };
  }
  function renderFlow(agent, flow) {
    saveDraft();
    active = { agentId: agent.id, flow };
    phase = uncertain.has(key(agent.id, flow.id)) ? "uncertain" : "open";
    controls = [];
    fields.replaceChildren();
    message("");
    document.getElementById("question-agent").textContent = `${agent.project} · ${agent.role || "agent"}`;
    document.getElementById("question-title").textContent = flow.title || "Your answer is needed";
    document.getElementById("question-source").textContent = flow.source === "tool"
      ? "Your answer returns to the waiting tool. No extra agent message is added."
      : "This is a reopened Pi form. Your answer follows the same resume or follow-up behavior as answering in Pi.";
    const picker = document.getElementById("question-picker");
    picker.replaceChildren();
    picker.hidden = agent.questions.length < 2;
    for (const item of agent.questions) {
      const button = element("button", "quiet-button", item.title || "Question");
      button.type = "button";
      button.setAttribute("aria-pressed", String(item.id === flow.id));
      button.addEventListener("click", () => {
        const latest = agents.find((candidate) => candidate.id === agent.id);
        const next = latest?.questions?.find((candidate) => candidate.id === item.id);
        if (phase !== "sending" && next) renderFlow(latest, next);
      });
      picker.append(button);
    }
    const draft = drafts.get(key(agent.id, flow.id)) || {};
    for (const [index, question] of flow.questions.entries()) {
      const section = element("fieldset", "answer-question");
      const legend = element("legend", "", question.prompt);
      section.append(legend);
      if (question.required) section.append(element("p", "answer-hint", "An answer is requested. You can still skip this question, as in Pi."));
      if (question.type === "multi") section.append(element("p", "answer-hint", "Choose any that apply."));
      const saved = draft[question.id] || {};
      const inputs = [];
      for (const option of question.options || []) {
        const label = element("label", "answer-option");
        const input = element("input");
        input.type = question.type === "multi" ? "checkbox" : "radio";
        input.name = `answer-${index}`;
        input.value = option.value;
        input.checked = saved.values?.includes(option.value) || false;
        const copy = element("span", "answer-option-copy");
        const heading = element("span", "answer-option-title", option.label);
        if (option.recommended) heading.append(element("span", "recommended-badge", "Recommended"));
        copy.append(heading);
        if (option.description) copy.append(element("span", "answer-option-description", option.description));
        if (option.preview) copy.append(element("pre", "answer-preview", option.preview));
        label.append(input, copy);
        section.append(label);
        inputs.push(input);
      }
      const custom = textArea("Your own answer", "Optional alternative to the choices above", saved.customText);
      const note = textArea("Note", "Optional context for your answer", saved.note);
      if (question.type !== "multi") {
        custom.input.addEventListener("input", () => {
          if (custom.input.value.trim()) for (const input of inputs) input.checked = false;
        });
        for (const input of inputs) input.addEventListener("change", () => { if (input.checked) custom.input.value = ""; });
      }
      const extras = element("details", "answer-extras");
      extras.open = Boolean(saved.customText || saved.note);
      extras.append(element("summary", "", "Write an answer or add a note"), custom.label, note.label);
      const clear = element("button", "quiet-button answer-clear", "Clear this answer");
      clear.type = "button";
      clear.addEventListener("click", () => {
        for (const input of inputs) input.checked = false;
        custom.input.value = "";
        note.input.value = "";
      });
      section.append(extras, clear);
      fields.append(section);
      controls.push({ id: question.id, options: inputs, custom: custom.input, note: note.input });
    }
    if (phase === "uncertain") message("Delivery could not be confirmed. Check this question in Pi; it cannot be sent again here.", true);
    setDisabled();
  }
  function open(agent) {
    if (phase === "sending") {
      if (!dialog.open) dialog.showModal();
      return;
    }
    const current = agents.find((candidate) => candidate.id === agent.id) || agent;
    const flow = current.questions?.[0];
    if (!flow || !online) return;
    if (active?.agentId !== current.id || active.flow.id !== flow.id || phase !== "open") renderFlow(current, flow);
    if (!dialog.open) dialog.showModal();
  }
  async function send(response) {
    if (!active || !online || phase !== "open") return;
    if (!pending()) {
      phase = "closed";
      message("This question is no longer waiting for an answer.", true);
      setDisabled();
      return;
    }
    saveDraft();
    const target = active;
    const body = JSON.stringify({ agentId: target.agentId, questionId: target.flow.id, response });
    if (new TextEncoder().encode(body).length > 65536) {
      message("This answer is too long. Shorten the text before sending.", true);
      return;
    }
    phase = "sending";
    message("Sending your answer to Pi…");
    setDisabled();
    try {
      const result = await fetch("/api/answer", {
        method: "POST",
        headers: { Authorization: `Bearer ${getToken()}`, "Content-Type": "application/json" },
        body,
        signal: AbortSignal.timeout(8000),
      });
      const data = await result.json().catch(() => ({}));
      if (result.ok && data.ok === true) {
        phase = "done";
        drafts.delete(key(target.agentId, target.flow.id));
        message(response.kind === "cancel" ? "Question cancelled in Pi." : "Answer received by Pi.");
      } else if (data.error === "invalid") {
        phase = pending() ? "open" : "closed";
        message(data.message || "Pi could not accept this answer. Check the choices and try again.", true);
      } else if (result.status === 409) {
        phase = "closed";
        message("This question has already ended. Your answer was not sent again.", true);
      } else {
        phase = "uncertain";
        message("Delivery could not be confirmed. Check Pi before answering again; this app will not retry.", true);
      }
    } catch {
      phase = "uncertain";
      message("Delivery could not be confirmed. Check Pi before answering again; this app will not retry.", true);
    } finally {
      if (phase === "uncertain") uncertain.add(key(target.agentId, target.flow.id));
      setDisabled();
      void refresh();
    }
  }
  form.addEventListener("submit", (event) => {
    event.preventDefault();
    void send({ kind: "answer", mode: "submit", answers: collect() });
  });
  cancel.addEventListener("click", () => {
    if (online && phase === "open" && confirm("Cancel this question in Pi? The agent will receive the normal cancelled result.")) void send({ kind: "cancel" });
  });
  document.getElementById("question-close").addEventListener("click", () => dialog.close());
  dialog.addEventListener("close", saveDraft);
  return {
    open,
    setAgent(agent) { selectedAgent = agent; renderLauncher(); },
    sync(nextAgents, connected) {
      agents = nextAgents;
      const reconnected = !online && connected;
      online = connected;
      selectedAgent = agents.find((agent) => agent.id === selectedAgent?.id);
      renderLauncher();
      if (active && phase === "open" && connected && !pending()) {
        saveDraft();
        phase = "closed";
        message("This question was answered in Pi or is no longer available.");
      } else if (reconnected && phase === "open") message("");
      setDisabled();
    },
  };
}
