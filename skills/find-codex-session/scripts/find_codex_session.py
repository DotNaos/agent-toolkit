#!/usr/bin/env python3
from __future__ import annotations

import argparse
import json
import re
from dataclasses import asdict, dataclass
from datetime import datetime
from pathlib import Path
CODEX_HOME = Path.home() / ".codex"
SESSION_INDEX = CODEX_HOME / "session_index.jsonl"
SESSIONS_ROOT = CODEX_HOME / "sessions"
SHELL_SNAPSHOTS_ROOT = CODEX_HOME / "shell_snapshots"
STOPWORDS = {
    "chat",
    "current",
    "find",
    "fresh",
    "latest",
    "match",
    "name",
    "not",
    "query",
    "real",
    "recent",
    "session",
    "skill",
    "thread",
}


@dataclass
class SessionMatch:
    id: str
    thread_name: str
    updated_at: str
    session_file: str | None
    shell_snapshots: list[str]
    score: int = 0
    transcript_score: int = 0
    evidence: list[str] | None = None


@dataclass(frozen=True)
class SearchSpec:
    query: str
    context: list[str]
    terms: list[str]
    phrases: list[str]


def parse_args() -> argparse.Namespace:
    parser = argparse.ArgumentParser(description="Find local Codex session files quickly.")
    parser.add_argument("--query", help="Thread name or session id fragment to match.")
    parser.add_argument("--session-id", help="Exact or partial session id.")
    parser.add_argument("--recent", action="store_true", help="Show most recent sessions.")
    parser.add_argument("--limit", type=int, default=5, help="Maximum matches to print.")
    parser.add_argument(
        "--recent-window",
        type=int,
        default=5,
        help="How many freshest sessions to inspect first when using --query.",
    )
    parser.add_argument(
        "--context",
        action="append",
        default=[],
        help="Additional current-chat text to compare against transcript contents. Repeat as needed.",
    )
    parser.add_argument(
        "--candidate-limit",
        type=int,
        default=0,
        help="How many broader fallback candidates to inspect via transcript after the fresh-session pass. 0 means all.",
    )
    parser.add_argument("--json", action="store_true", help="Print JSON output.")
    return parser.parse_args()


def parse_iso(value: str) -> datetime:
    return datetime.fromisoformat(value.replace("Z", "+00:00"))


def find_session_file(session_id: str) -> str | None:
    if not SESSIONS_ROOT.exists():
        return None
    matches = sorted(SESSIONS_ROOT.glob(f"**/*{session_id}*.jsonl"))
    return str(matches[-1]) if matches else None


def find_shell_snapshots(session_id: str) -> list[str]:
    if not SHELL_SNAPSHOTS_ROOT.exists():
        return []
    return [str(path) for path in sorted(SHELL_SNAPSHOTS_ROOT.glob(f"{session_id}.*.sh"))]


def load_index() -> list[SessionMatch]:
    if not SESSION_INDEX.exists():
        raise FileNotFoundError(f"Missing session index: {SESSION_INDEX}")

    latest_by_id: dict[str, SessionMatch] = {}

    for raw_line in SESSION_INDEX.read_text().splitlines():
        if not raw_line.strip():
            continue
        row = json.loads(raw_line)
        session_id = row["id"]
        candidate = SessionMatch(
            id=session_id,
            thread_name=row.get("thread_name", ""),
            updated_at=row["updated_at"],
            session_file=find_session_file(session_id),
            shell_snapshots=find_shell_snapshots(session_id),
        )
        previous = latest_by_id.get(session_id)
        if previous is None or parse_iso(candidate.updated_at) >= parse_iso(previous.updated_at):
            latest_by_id[session_id] = candidate

    return sorted(latest_by_id.values(), key=lambda item: parse_iso(item.updated_at), reverse=True)


def score_match(candidate: SessionMatch, query: str) -> int:
    query_lower = query.lower()
    score = 0

    if candidate.id == query:
        score += 100
    elif query in candidate.id:
        score += 60

    name_lower = candidate.thread_name.lower()
    if name_lower == query_lower:
        score += 90
    elif query_lower in name_lower:
        score += 45

    words = normalize_query_terms(query)
    score += sum(5 for word in words if word in name_lower)
    return score


def filter_matches(sessions: list[SessionMatch], query: str | None, session_id: str | None) -> list[SessionMatch]:
    effective_query = session_id or query
    if not effective_query:
        return sessions

    ranked: list[SessionMatch] = []
    for session in sessions:
        score = score_match(session, effective_query)
        if score <= 0:
            continue
        session.score = score
        ranked.append(session)

    return sorted(ranked, key=lambda item: (item.score, parse_iso(item.updated_at)), reverse=True)


def normalize_query_terms(query: str) -> list[str]:
    return [
        part
        for part in re.split(r"[^a-z0-9]+", query.lower())
        if len(part) >= 3 and part not in STOPWORDS
    ]


def dedupe_preserving_order(items: list[str]) -> list[str]:
    seen: set[str] = set()
    ordered: list[str] = []
    for item in items:
        if item in seen:
            continue
        seen.add(item)
        ordered.append(item)
    return ordered


def build_search_spec(query: str, context: list[str]) -> SearchSpec:
    clean_context = [item.strip() for item in context if item and item.strip()]
    phrases = dedupe_preserving_order([query.strip().lower(), *[item.lower() for item in clean_context]])
    terms = dedupe_preserving_order(
        normalize_query_terms(query) + [term for item in clean_context for term in normalize_query_terms(item)]
    )
    return SearchSpec(query=query, context=clean_context, terms=terms, phrases=phrases)


def extract_transcript_texts(row: dict) -> list[str]:
    texts: list[str] = []
    row_type = row.get("type")
    payload = row.get("payload", {})

    if row_type == "response_item" and payload.get("type") == "message":
        if payload.get("role") not in {"assistant", "user"}:
            return texts
        for item in payload.get("content", []):
            if item.get("type") in {"input_text", "output_text"}:
                text = item.get("text")
                if isinstance(text, str) and text.strip():
                    texts.append(text)
        return texts

    if row_type == "event_msg":
        event_type = payload.get("type")
        if event_type == "user_message":
            text = payload.get("message")
            if isinstance(text, str) and text.strip():
                texts.append(text)
        elif event_type == "thread_name_updated":
            text = payload.get("thread_name")
            if isinstance(text, str) and text.strip():
                texts.append(text)
        return texts

    return texts


def snippet_from_text(text: str, terms: list[str]) -> str | None:
    lowered = text.lower()
    positions = [lowered.find(term) for term in terms if term and lowered.find(term) != -1]
    if not positions:
        return None
    start = max(0, min(positions) - 60)
    end = min(len(text), max(positions) + 160)
    snippet = " ".join(text[start:end].split())
    return snippet


def inspect_transcript(match: SessionMatch, search: SearchSpec) -> None:
    if not match.session_file:
        match.evidence = []
        match.transcript_score = 0
        return

    transcript_path = Path(match.session_file)
    if not transcript_path.exists():
        match.evidence = []
        match.transcript_score = 0
        return

    terms = search.terms
    phrases = [phrase for phrase in search.phrases if phrase]
    if not terms and not phrases:
        match.evidence = []
        match.transcript_score = 0
        return

    evidence_candidates: list[tuple[int, str]] = []
    transcript_score = 0

    for raw_line in transcript_path.read_text().splitlines():
        if not raw_line.strip():
            continue
        try:
            row = json.loads(raw_line)
        except json.JSONDecodeError:
            continue

        strings = extract_transcript_texts(row)
        for text in strings:
            lowered = text.lower()
            phrase_hits = sum(1 for phrase in phrases if phrase in lowered)
            if phrase_hits:
                transcript_score += phrase_hits * 120
            term_hits = sum(1 for term in terms if term in lowered)
            if not phrase_hits and not term_hits:
                continue
            if not phrase_hits and term_hits == 1 and match.score == 0:
                continue
            transcript_score += term_hits * 10
            snippet_terms = terms or [part for phrase in phrases for part in normalize_query_terms(phrase)]
            snippet = snippet_from_text(text, snippet_terms)
            if snippet:
                evidence_candidates.append((term_hits + (phrase_hits * 12), snippet))

    match.transcript_score = transcript_score
    best_evidence: list[str] = []
    seen: set[str] = set()
    for _, snippet in sorted(evidence_candidates, key=lambda item: item[0], reverse=True):
        if snippet in seen:
            continue
        seen.add(snippet)
        best_evidence.append(snippet)
        if len(best_evidence) >= 3:
            break
    match.evidence = best_evidence


def rerank_with_transcript(matches: list[SessionMatch], search: SearchSpec, candidate_limit: int) -> list[SessionMatch]:
    inspect_limit = len(matches) if candidate_limit == 0 else max(1, candidate_limit)
    inspected = matches[:inspect_limit]
    uninspected = matches[inspect_limit:]

    for match in inspected:
        inspect_transcript(match, search)

    ranked = sorted(
        inspected,
        key=lambda item: (item.transcript_score, item.score, parse_iso(item.updated_at)),
        reverse=True,
    )
    return ranked + uninspected


def rank_recent_candidates(sessions: list[SessionMatch], query: str, recent_window: int) -> list[SessionMatch]:
    recent_candidates: list[SessionMatch] = []
    for session in sessions[: max(1, recent_window)]:
        session.score = score_match(session, query)
        recent_candidates.append(session)
    return sorted(recent_candidates, key=lambda item: (item.score, parse_iso(item.updated_at)), reverse=True)


def rank_broad_candidates(sessions: list[SessionMatch], query: str) -> list[SessionMatch]:
    ranked: list[SessionMatch] = []
    for session in sessions:
        session.score = score_match(session, query)
        ranked.append(session)
    return sorted(ranked, key=lambda item: (item.score, parse_iso(item.updated_at)), reverse=True)


def transcript_hits(matches: list[SessionMatch]) -> list[SessionMatch]:
    return [match for match in matches if match.transcript_score > 0]


def print_human(matches: list[SessionMatch]) -> None:
    if not matches:
        print("No matching session found.")
        return
    for index, match in enumerate(matches, start=1):
        print(f"[{index}] {match.thread_name}")
        print(f"  id: {match.id}")
        print(f"  updated_at: {match.updated_at}")
        print(f"  session_file: {match.session_file or '-'}")
        if match.shell_snapshots:
            print("  shell_snapshots:")
            for snapshot in match.shell_snapshots:
                print(f"    - {snapshot}")
        else:
            print("  shell_snapshots: -")
        if match.score:
            print(f"  score: {match.score}")
        if match.transcript_score:
            print(f"  transcript_score: {match.transcript_score}")
        if match.evidence:
            print("  evidence:")
            for snippet in match.evidence:
                print(f"    - {snippet}")
        print()


def main() -> int:
    args = parse_args()
    sessions = load_index()

    if not (args.query or args.session_id or args.recent):
        raise SystemExit("Use --recent, --query, or --session-id.")

    matches = filter_matches(sessions, args.query, args.session_id)
    if args.recent and not (args.query or args.session_id):
        matches = sessions
    elif args.query:
        search = build_search_spec(args.query, args.context)
        recent_matches = rerank_with_transcript(
            rank_recent_candidates(sessions, args.query, args.recent_window),
            search,
            args.recent_window,
        )
        recent_hits = transcript_hits(recent_matches)
        if recent_hits:
            matches = recent_hits
        else:
            matches = transcript_hits(
                rerank_with_transcript(rank_broad_candidates(sessions, args.query), search, args.candidate_limit)
            )

    matches = matches[: max(1, args.limit)]

    if args.json:
        print(json.dumps([asdict(match) for match in matches], indent=2))
    else:
        print_human(matches)

    return 0


if __name__ == "__main__":
    raise SystemExit(main())
