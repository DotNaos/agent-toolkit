---
name: strict-ui-gatekeeping
description: Design, review, or refactor UI and system flows so invalid screens, actions, renderers, and private data states are unreachable unless their preconditions have been proven. Use for state-machine UI design, access/auth/resource gating, dashboard or graph renderer safety, stale cache or URL-derived state risks, and prompts asking to make wrong UI states structurally impossible rather than handling them late with warnings or empty states.
---

# Strict UI Gatekeeping

Use this skill to turn a fragile UI flow into a state-based design where later screens and actions cannot appear until earlier checks have produced evidence.

The central rule is:

```text
ScreenVisible(S) => Preconditions(S)
ActionEnabled(A) => Preconditions(A)
RendererVisible(R) => Preconditions(R)
```

If preconditions are missing, show only `Checking`, `Blocked(reason)`, or a clearly marked `LastSafeState`. Do not render the later screen, action, graph, dashboard, detail view, or builder.

## Workflow

1. Name the bad state.
   - State what the user can currently see or do that should be impossible.
   - Identify the false assumption that makes it possible.
   - Treat late warnings, fallback nodes, and empty states as symptoms, not fixes, when a precondition is missing.

2. Define the state machine.
   - List real states that can be reached only through checks.
   - List derived states that must not be treated as facts.
   - Mark states that must never be defaults.
   - Pick the earliest safe visible state.

3. Define evidence.
   - Every proven state needs an explicit evidence object.
   - Name which request, response, check, or local invariant creates each evidence object.
   - Do not allow `Ready`, `Loaded`, `Empty`, `Authenticated`, `ResourceAccessVerified`, or similar states without evidence.

4. Put gates at the earliest useful edge.
   - Define preconditions for every screen.
   - Define preconditions for every action.
   - Define preconditions for every renderer or builder.
   - Block before transition into the later state, not inside the later renderer.

5. Define allowed transitions.
   - For each transition, state required evidence.
   - State what transitions are blocked while evidence is missing.
   - Ensure failed gates go to `Blocked(reason)` or stay at `LastSafeState`.

6. Define UI truth rules.
   - State which UI claims require which proven state.
   - State what may be shown before checks complete.
   - State what must not be claimed from URL, cache, previous state, or failed requests.

7. Write invariants and a proof sketch.
   - Express invariants in implication form.
   - Show that if the UI displays X, evidence Z must exist.
   - Show that a later renderer cannot display errors that should have been blocked by earlier gates.

8. Derive code and test consequences.
   - Identify types, components, loaders, routes, or service boundaries that should carry evidence.
   - Identify tests for missing auth, denied access, missing resources, stale cache, empty success, failed load, and backend mismatch.

## Gate Rules

Do not let later renderers model earlier gate failures.

Examples of errors that usually belong at a gate edge:

- authentication required
- access denied
- resource missing
- course access missing
- backend mismatch
- required capability missing

If one of these is a precondition for a screen or renderer, the renderer must not know how to display it. The gate should block before the renderer exists.

Use this rule:

```text
RendererVisible(R)
=> Preconditions(R) proven
=> Gate errors for R excluded
```

Therefore this should be impossible:

```text
RendererVisible(R)
AND R shows an error that violates a precondition of R
```

## Evidence Rules

Do not infer proven state from untrusted or stale signals.

Forbidden inferences:

- URL contains an id => the resource exists.
- Cache contains data => access is still allowed.
- Previous state says logged in => session is still valid.
- Empty array returned or present locally => there is truly no data.
- Loaded title exists => the current user may see that resource.
- A failed request included metadata => private resource information may be shown.

Use explicit evidence objects, for example:

```text
Authenticated(user) => AuthEvidence
ResourceAccessVerified(resource) => AccessEvidence
Loaded(data) => LoadEvidence
Empty => LoadSuccessEvidence(emptyResult)
Ready => CheckEvidence(allRequiredChecksPassed)
```

Private or unproven information must not be leaked. Without proven preconditions, do not confirm that a resource exists, what it is called, whether data exists, how many items exist, or which actions would be available.

## Output Format

When producing a design or review, use this shape:

1. Problem abstraction
2. Bad state
3. Why the current approach fails
4. State machine
5. ASCII state transition diagram
6. Evidence objects
7. Preconditions and gates
8. Earliest allowed display
9. Renderer rules
10. UI invariants
11. Proof sketch
12. Allowed error cases
13. Disallowed error cases
14. Consequences for code, tests, and documentation

Keep the answer concrete. Tie every screen, action, renderer, and user-visible claim to named evidence and named gates.
