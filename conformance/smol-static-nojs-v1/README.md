# smol/static-nojs-v1 conformance corpus

This corpus documents the current `smol/static-nojs-v1` contract with two
fixture tiers.

Portable validation fixtures are self-contained site directories. They use
`action: "load"` when the contract under test is config/content validation and
should not depend on any smol starter theme. The page-navigation invalid
fixtures are in this tier. The same navigation failures are also covered through
`BuildSite` by the normal Go navigation tests.

Reference output-shape fixtures use shipped smol starters plus fixture overlays.
They check byte boundaries, generated navigation placement, route-relative
links, manifest region provenance, manifest absence in flat modes, and Gemini
capsule shape. These fixtures are intentionally smol-reference-specific.

Check fixtures use `action: "check"` to verify already-built output through the
public `smol check` path. Starter-backed check fixtures build first, then check
`public/`; direct check fixtures provide the already-built file under
`check_path`.

Check fixtures may include a `tamper` object with `file`, `old`, and `new`.
Tamper applies only to `action: "check"` after any starter-backed build and
before `smol check`; `old` must occur exactly once. Tampered fixtures should use
benign byte edits that preserve static-policy validity so the expected failure
comes from the intended conformance check.

Invalid fixtures should be single-defect cases so the expected error substring
identifies the intended failure.
