# agent-chat

Use this skill for local agent-to-agent inbox workflows with a lightweight daemon.

## Build

```bash
go build -o bin/agent-chat ./cmd/agent-chat
```

## Core commands

Start daemon:

```bash
agent-chat daemon
```

Send a message:

```bash
agent-chat send --to agent-b --thread collab --message "Hallo von agent-a"
```

Wait for next message:

```bash
agent-chat wait --agent agent-b --thread collab
```

Always-on worker:

```bash
agent-chat watch --agent agent-b --thread collab --auto-ack
```

Use `agent-chat --help` for full flags and options.
