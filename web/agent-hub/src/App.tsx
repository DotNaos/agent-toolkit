import { FormEvent, useCallback, useEffect, useMemo, useRef, useState } from "react";
import { AlertTriangle, Loader2, Plus, Send, Zap } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { Checkbox } from "@/components/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/components/ui/dialog";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/components/ui/select";
import { Textarea } from "@/components/ui/textarea";

type Conversation = { id: string; name: string; created_at: string };
type Message = {
  id: string;
  conversation_id: string;
  from_id: string;
  to_id?: string;
  kind: "text" | "system";
  body: string;
  created_at: string;
};

type Approval = {
  id: string;
  conversation_id: string;
  agent_id: string;
  title: string;
  description: string;
  schema_json: string;
  risk_level: string;
  status: "pending" | "approved" | "rejected" | "selected" | "expired";
  expires_at: string;
  created_at: string;
};

type EventLogEntry = { id: string; text: string };

type ApprovalField = {
  key: string;
  type: "enum" | "array-enum" | "text";
  values?: string[];
};

const ownerId = "owner";

async function api<T>(path: string, init?: RequestInit): Promise<T> {
  const response = await fetch(path, {
    headers: {
      "Content-Type": "application/json",
      ...(init?.headers || {}),
    },
    ...init,
  });

  const payload = await response.json().catch(() => ({}));
  if (!response.ok) {
    throw new Error(payload.message || `Request failed (${response.status})`);
  }
  return payload as T;
}

function parseApprovalFields(schemaJSON: string): ApprovalField[] {
  try {
    const schema = JSON.parse(schemaJSON || "{}");
    const props = schema.properties || {};
    const fields: ApprovalField[] = [];
    for (const [key, value] of Object.entries<any>(props)) {
      if (Array.isArray(value.enum)) {
        fields.push({ key, type: "enum", values: value.enum.map(String) });
        continue;
      }
      if (value.type === "array" && value.items && Array.isArray(value.items.enum)) {
        fields.push({ key, type: "array-enum", values: value.items.enum.map(String) });
        continue;
      }
      fields.push({ key, type: "text" });
    }
    return fields;
  } catch {
    return [];
  }
}

export default function App() {
  const [conversations, setConversations] = useState<Conversation[]>([]);
  const [conversationId, setConversationId] = useState<string>("");
  const [messages, setMessages] = useState<Message[]>([]);
  const [pendingApprovals, setPendingApprovals] = useState<Approval[]>([]);
  const [eventStatus, setEventStatus] = useState("connecting");
  const [eventLog, setEventLog] = useState<EventLogEntry[]>([]);
  const [fromId, setFromId] = useState(ownerId);
  const [toId, setToId] = useState("");
  const [messageBody, setMessageBody] = useState("");
  const [dispatchAgent, setDispatchAgent] = useState("agent-a");
  const [dispatchAction, setDispatchAction] = useState("read");
  const [dispatchPrompt, setDispatchPrompt] = useState("");
  const [isBooting, setIsBooting] = useState(true);

  const timelineRef = useRef<HTMLDivElement>(null);
  const eventSourceRef = useRef<EventSource | null>(null);

  const activeConversation = useMemo(
    () => conversations.find((item) => item.id === conversationId) ?? null,
    [conversations, conversationId]
  );

  const activeApproval = pendingApprovals[0] ?? null;
  const approvalFields = useMemo(() => parseApprovalFields(activeApproval?.schema_json ?? ""), [activeApproval?.schema_json]);

  const [approvalEnumValues, setApprovalEnumValues] = useState<Record<string, string>>({});
  const [approvalArrayValues, setApprovalArrayValues] = useState<Record<string, string[]>>({});
  const [approvalTextValues, setApprovalTextValues] = useState<Record<string, string>>({});

  const logEvent = useCallback((text: string) => {
    setEventLog((prev) => [{ id: crypto.randomUUID(), text: `${new Date().toLocaleTimeString()}  ${text}` }, ...prev].slice(0, 120));
  }, []);

  const loadConversations = useCallback(async () => {
    const payload = await api<{ conversations: Conversation[] }>("/v1/conversations");
    setConversations(payload.conversations ?? []);
    return payload.conversations ?? [];
  }, []);

  const loadMessages = useCallback(async (id: string) => {
    if (!id) return;
    const payload = await api<{ messages: Message[] }>(`/v1/messages?conversation_id=${encodeURIComponent(id)}`);
    setMessages(payload.messages ?? []);
  }, []);

  const loadPendingApprovals = useCallback(async (id: string) => {
    if (!id) return;
    const payload = await api<{ approvals: Approval[] }>(`/v1/approvals/pending?conversation_id=${encodeURIComponent(id)}`);
    setPendingApprovals(payload.approvals ?? []);
  }, []);

  const ensureConversation = useCallback(async () => {
    const existing = await loadConversations();
    if (existing.length > 0) {
      setConversationId(existing[0].id);
      return existing[0].id;
    }

    const created = await api<{ conversation_id: string }>("/v1/conversations", {
      method: "POST",
      body: JSON.stringify({
        name: "Main Group",
        participants: [
          { type: "human", id: ownerId },
          { type: "agent", id: "agent-a" },
          { type: "agent", id: "agent-b" },
        ],
      }),
    });

    const refreshed = await loadConversations();
    const id = created.conversation_id || refreshed[0]?.id;
    if (id) {
      setConversationId(id);
    }
    return id;
  }, [loadConversations]);

  const openEventStream = useCallback(
    (id: string) => {
      if (!id) return;
      eventSourceRef.current?.close();

      const source = new EventSource(`/v1/events/stream?conversation_id=${encodeURIComponent(id)}`);
      eventSourceRef.current = source;
      setEventStatus("connecting");

      source.onopen = () => setEventStatus("connected");
      source.onerror = () => setEventStatus("reconnecting");

      source.addEventListener("message.created", (event) => {
        const data = JSON.parse(event.data);
        if (data.message) {
          setMessages((prev) => [...prev, data.message as Message]);
        }
        logEvent("message.created");
      });

      source.addEventListener("approval.requested", (event) => {
        const data = JSON.parse(event.data);
        if (data.approval) {
          setPendingApprovals((prev) => {
            const filtered = prev.filter((item) => item.id !== data.approval.id);
            return [...filtered, data.approval as Approval];
          });
        }
        logEvent("approval.requested");
      });

      source.addEventListener("approval.resolved", (event) => {
        const data = JSON.parse(event.data);
        if (data.approval?.id) {
          setPendingApprovals((prev) => prev.filter((item) => item.id !== data.approval.id));
        }
        logEvent("approval.resolved");
      });

      source.addEventListener("agent.status", (event) => {
        const data = JSON.parse(event.data);
        logEvent(`agent.status ${data.agent_id ?? "?"} -> ${data.status ?? "?"}`);
      });
    },
    [logEvent]
  );

  useEffect(() => {
    let active = true;

    async function boot() {
      try {
        const id = await ensureConversation();
        if (!active || !id) return;
        await loadMessages(id);
        await loadPendingApprovals(id);
        openEventStream(id);
      } catch (error) {
        logEvent(`boot error: ${(error as Error).message}`);
      } finally {
        if (active) {
          setIsBooting(false);
        }
      }
    }

    void boot();

    return () => {
      active = false;
      eventSourceRef.current?.close();
    };
  }, [ensureConversation, loadMessages, loadPendingApprovals, openEventStream, logEvent]);

  useEffect(() => {
    if (!conversationId) return;
    void loadMessages(conversationId);
    void loadPendingApprovals(conversationId);
    openEventStream(conversationId);
  }, [conversationId, loadMessages, loadPendingApprovals, openEventStream]);

  useEffect(() => {
    if (timelineRef.current) {
      timelineRef.current.scrollTop = timelineRef.current.scrollHeight;
    }
  }, [messages]);

  useEffect(() => {
    if (!activeApproval) {
      setApprovalEnumValues({});
      setApprovalArrayValues({});
      setApprovalTextValues({});
      return;
    }
    const enumDefaults: Record<string, string> = {};
    const arrayDefaults: Record<string, string[]> = {};
    const textDefaults: Record<string, string> = {};

    for (const field of approvalFields) {
      if (field.type === "enum") {
        enumDefaults[field.key] = field.values?.[0] ?? "";
      } else if (field.type === "array-enum") {
        arrayDefaults[field.key] = [];
      } else {
        textDefaults[field.key] = "";
      }
    }

    setApprovalEnumValues(enumDefaults);
    setApprovalArrayValues(arrayDefaults);
    setApprovalTextValues(textDefaults);
  }, [activeApproval, approvalFields]);

  async function onSendMessage(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!conversationId || !messageBody.trim()) return;

    try {
      await api("/v1/messages", {
        method: "POST",
        body: JSON.stringify({
          conversation_id: conversationId,
          from_id: fromId.trim() || ownerId,
          to_id: toId.trim() || null,
          body: messageBody,
          kind: "text",
        }),
      });
      setMessageBody("");
      setToId("");
    } catch (error) {
      logEvent(`send failed: ${(error as Error).message}`);
    }
  }

  async function onCreateConversation() {
    const name = window.prompt("Conversation name", `Room ${conversations.length + 1}`);
    if (!name) return;

    try {
      const created = await api<{ conversation_id: string }>("/v1/conversations", {
        method: "POST",
        body: JSON.stringify({
          name,
          participants: [
            { type: "human", id: ownerId },
            { type: "agent", id: "agent-a" },
            { type: "agent", id: "agent-b" },
          ],
        }),
      });
      await loadConversations();
      setConversationId(created.conversation_id);
    } catch (error) {
      logEvent(`create conversation failed: ${(error as Error).message}`);
    }
  }

  async function onDispatch(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    if (!conversationId || !dispatchPrompt.trim()) return;

    try {
      const payload = await api<{ dispatch_id: string; status: string }>(`/v1/agents/${encodeURIComponent(dispatchAgent)}/dispatch`, {
        method: "POST",
        body: JSON.stringify({
          conversation_id: conversationId,
          prompt: dispatchPrompt,
          metadata: { action: dispatchAction },
        }),
      });
      logEvent(`dispatch ${payload.dispatch_id} -> ${payload.status}`);
      setDispatchPrompt("");
    } catch (error) {
      logEvent(`dispatch failed: ${(error as Error).message}`);
    }
  }

  async function submitApproval(decision: "reject" | "select") {
    if (!activeApproval) return;

    const payload: Record<string, any> = {};
    for (const field of approvalFields) {
      if (field.type === "enum") {
        payload[field.key] = approvalEnumValues[field.key] ?? "";
      } else if (field.type === "array-enum") {
        payload[field.key] = approvalArrayValues[field.key] ?? [];
      } else {
        payload[field.key] = approvalTextValues[field.key] ?? "";
      }
    }

    const finalDecision = decision === "reject" ? "reject" : payload.decision || "approve";

    try {
      await api(`/v1/approvals/${activeApproval.id}/respond`, {
        method: "POST",
        body: JSON.stringify({
          human_id: ownerId,
          decision: finalDecision,
          payload,
        }),
      });
      setPendingApprovals((prev) => prev.filter((item) => item.id !== activeApproval.id));
      logEvent(`approval ${activeApproval.id} resolved (${finalDecision})`);
    } catch (error) {
      logEvent(`approval failed: ${(error as Error).message}`);
    }
  }

  function setArrayValue(key: string, value: string, checked: boolean) {
    setApprovalArrayValues((prev) => {
      const current = new Set(prev[key] ?? []);
      if (checked) {
        current.add(value);
      } else {
        current.delete(value);
      }
      return { ...prev, [key]: Array.from(current) };
    });
  }

  if (isBooting) {
    return (
      <div className="flex min-h-screen items-center justify-center gap-3 text-lg font-medium">
        <Loader2 className="h-6 w-6 animate-spin" />
        Booting Agent Hub...
      </div>
    );
  }

  return (
    <div className="relative min-h-screen overflow-hidden bg-noise">
      <div className="mx-auto grid w-full max-w-[1600px] grid-cols-1 gap-4 p-4 lg:grid-cols-[320px_minmax(520px,1fr)_320px]">
        <Card className="border-foreground/10 bg-card/85">
          <CardHeader>
            <p className="text-xs uppercase tracking-[0.26em] text-muted-foreground">Agent Toolkit</p>
            <CardTitle>Agent Hub</CardTitle>
            <CardDescription>Realtime web group chat with mandatory human approval on risky actions.</CardDescription>
          </CardHeader>
          <CardContent className="space-y-5">
            <section>
              <div className="mb-2 flex items-center justify-between">
                <h3 className="font-display text-lg">Conversations</h3>
                <Button variant="outline" size="sm" onClick={onCreateConversation}>
                  <Plus className="mr-1 h-4 w-4" />
                  New
                </Button>
              </div>
              <div className="space-y-2">
                {conversations.map((conversation) => (
                  <Button
                    key={conversation.id}
                    variant={conversation.id === conversationId ? "default" : "outline"}
                    className="w-full justify-start"
                    onClick={() => setConversationId(conversation.id)}
                  >
                    {conversation.name}
                  </Button>
                ))}
              </div>
            </section>

            <section className="space-y-3">
              <h3 className="font-display text-lg">Agent Dispatch</h3>
              <form className="space-y-3" onSubmit={onDispatch}>
                <div className="space-y-1.5">
                  <Label htmlFor="dispatch-agent">Agent</Label>
                  <Input id="dispatch-agent" value={dispatchAgent} onChange={(event) => setDispatchAgent(event.target.value)} />
                </div>
                <div className="space-y-1.5">
                  <Label>Action Type</Label>
                  <Select value={dispatchAction} onValueChange={setDispatchAction}>
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      <SelectItem value="read">read</SelectItem>
                      <SelectItem value="write">write</SelectItem>
                      <SelectItem value="deploy">deploy</SelectItem>
                    </SelectContent>
                  </Select>
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="dispatch-prompt">Prompt</Label>
                  <Textarea
                    id="dispatch-prompt"
                    rows={4}
                    value={dispatchPrompt}
                    onChange={(event) => setDispatchPrompt(event.target.value)}
                    placeholder="Describe what the agent should do..."
                  />
                </div>
                <Button type="submit" className="w-full">
                  <Zap className="mr-2 h-4 w-4" />
                  Dispatch
                </Button>
              </form>
            </section>
          </CardContent>
        </Card>

        <Card className="grid min-h-[82vh] grid-rows-[auto_1fr_auto] border-foreground/10 bg-card/88">
          <CardHeader className="flex-row items-center justify-between border-b border-border/60">
            <div>
              <p className="text-xs uppercase tracking-[0.22em] text-muted-foreground">Current room</p>
              <CardTitle>{activeConversation?.name ?? "No conversation"}</CardTitle>
            </div>
            <Badge variant={eventStatus === "connected" ? "default" : "secondary"}>{eventStatus}</Badge>
          </CardHeader>

          <CardContent className="overflow-hidden p-0">
            <div ref={timelineRef} className="h-full space-y-3 overflow-y-auto p-5">
              {messages.map((message) => (
                <article key={message.id} className="animate-fade-in rounded-lg border border-border/70 bg-background/90 p-3">
                  <header className="mb-1 flex items-center justify-between gap-3 text-sm">
                    <strong className="font-semibold">
                      {message.from_id}
                      {message.to_id ? ` → ${message.to_id}` : ""}
                    </strong>
                    <time className="text-xs text-muted-foreground">{new Date(message.created_at).toLocaleTimeString()}</time>
                  </header>
                  <p className="whitespace-pre-wrap text-sm leading-relaxed">{message.body}</p>
                </article>
              ))}
            </div>
          </CardContent>

          <CardContent className="border-t border-border/60 pt-4">
            <form className="space-y-3" onSubmit={onSendMessage}>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2">
                <div className="space-y-1.5">
                  <Label htmlFor="from-input">From</Label>
                  <Input id="from-input" value={fromId} onChange={(event) => setFromId(event.target.value)} />
                </div>
                <div className="space-y-1.5">
                  <Label htmlFor="to-input">To (optional)</Label>
                  <Input id="to-input" value={toId} onChange={(event) => setToId(event.target.value)} placeholder="agent-a" />
                </div>
              </div>
              <div className="space-y-1.5">
                <Label htmlFor="message-body">Message</Label>
                <Textarea
                  id="message-body"
                  rows={3}
                  value={messageBody}
                  onChange={(event) => setMessageBody(event.target.value)}
                  placeholder="Write to agents without any terminal polling loops..."
                />
              </div>
              <Button type="submit">
                <Send className="mr-2 h-4 w-4" />
                Send message
              </Button>
            </form>
          </CardContent>
        </Card>

        <Card className="border-foreground/10 bg-card/86">
          <CardHeader>
            <CardTitle>Live Events</CardTitle>
            <CardDescription>SSE signal log for message, approval, and agent status events.</CardDescription>
          </CardHeader>
          <CardContent className="max-h-[74vh] space-y-2 overflow-y-auto">
            {eventLog.length === 0 && <p className="text-sm text-muted-foreground">No events yet.</p>}
            {eventLog.map((entry) => (
              <div key={entry.id} className="rounded-md border border-border/70 bg-background p-2 text-xs">
                {entry.text}
              </div>
            ))}
          </CardContent>
        </Card>
      </div>

      <Dialog
        open={Boolean(activeApproval)}
        onOpenChange={(open) => {
          if (!open && activeApproval) {
            // mandatory human-in-the-loop: keep modal open while pending approval exists
            return;
          }
        }}
      >
        <DialogContent
          onEscapeKeyDown={(event) => event.preventDefault()}
          onPointerDownOutside={(event) => event.preventDefault()}
        >
          <DialogHeader>
            <div className="mb-1 flex items-center gap-2">
              <AlertTriangle className="h-5 w-5 text-destructive" />
              <Badge variant="secondary">Approval Required</Badge>
            </div>
            <DialogTitle>{activeApproval?.title}</DialogTitle>
            <DialogDescription>{activeApproval?.description}</DialogDescription>
          </DialogHeader>

          <div className="space-y-4">
            {approvalFields.map((field) => {
              if (field.type === "enum") {
                return (
                  <div key={field.key} className="space-y-1.5">
                    <Label>{field.key}</Label>
                    <Select
                      value={approvalEnumValues[field.key] ?? ""}
                      onValueChange={(value) => setApprovalEnumValues((prev) => ({ ...prev, [field.key]: value }))}
                    >
                      <SelectTrigger>
                        <SelectValue />
                      </SelectTrigger>
                      <SelectContent>
                        {(field.values ?? []).map((value) => (
                          <SelectItem key={value} value={value}>
                            {value}
                          </SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </div>
                );
              }

              if (field.type === "array-enum") {
                return (
                  <div key={field.key} className="space-y-2">
                    <Label>{field.key}</Label>
                    <div className="space-y-2 rounded-md border border-border/70 bg-muted/30 p-3">
                      {(field.values ?? []).map((value) => {
                        const checked = (approvalArrayValues[field.key] ?? []).includes(value);
                        return (
                          <label key={value} className="flex items-center gap-2 text-sm">
                            <Checkbox checked={checked} onCheckedChange={(state) => setArrayValue(field.key, value, state === true)} />
                            <span>{value}</span>
                          </label>
                        );
                      })}
                    </div>
                  </div>
                );
              }

              return (
                <div key={field.key} className="space-y-1.5">
                  <Label>{field.key}</Label>
                  <Textarea
                    rows={2}
                    value={approvalTextValues[field.key] ?? ""}
                    onChange={(event) => setApprovalTextValues((prev) => ({ ...prev, [field.key]: event.target.value }))}
                  />
                </div>
              );
            })}
          </div>

          <DialogFooter>
            <Button type="button" variant="destructive" onClick={() => void submitApproval("reject")}>Reject</Button>
            <Button type="button" onClick={() => void submitApproval("select")}>Submit Response</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  );
}
