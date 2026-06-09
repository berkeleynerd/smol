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
links, manifest absence in flat modes, and Gemini capsule shape. These fixtures
are intentionally smol-reference-specific.

Invalid fixtures should be single-defect cases so the expected error substring
identifies the intended failure.
