# SMOL ATTESTED MANIFEST V1

`SMOL ATTESTED MANIFEST V1` is a base64-encoded JSON comment embedded in nested
HTML output.

When signing is enabled, the manifest is inside the HTML payload that `attest`
/ `attested-html` signs. It is covered by the whole-page signature in signed
output, but it is not itself a separate signature format. Unsigned nested HTML
builds still include the manifest comment without a signature.

`flat-xhtml-v1` and `flat-gemini-v1` outputs do not currently emit this
manifest.

## Wrapper

The comment wrapper is:

```html
<!--SMOL ATTESTED MANIFEST V1
BASE64-ENCODED-JSON
SMOL ATTESTED MANIFEST END-->
```

The base64 text is wrapped for readability. Decoding it yields a JSON object.

## Fields

Current manifests emit these fields:

```json
{
  "format": "smol-attested-manifest-v1",
  "profile": "smol/static-nojs-v1",
  "generator": {
    "name": "smol",
    "version": "0.1.0"
  },
  "publisher": {
    "name": "Example Publisher",
    "origin": "https://example.org",
    "url": "https://example.org"
  },
  "page": {
    "kind": "page",
    "title": "Example",
    "canonical_url": "https://example.org/",
    "published_utc": "",
    "updated_utc": ""
  },
  "policy": {
    "javascript": false,
    "external_resources": false,
    "self_contained": true
  },
  "allowed_origins": ["https://example.org"],
  "resources": []
}
```

`profile` uses the canonical slash-form identifier `smol/static-nojs-v1`.

## Resources

`resources` records embedded local resources included in the generated page.
Current resource entries use:

```json
{
  "kind": "css",
  "source": "theme:default/assets/style.css",
  "embedded_as": "inline-style",
  "sha256": "..."
}
```

```json
{
  "kind": "image",
  "source": "content:pages/index/assets/hero.png",
  "embedded_as": "data-url",
  "sha256": "..."
}
```

Supported current resource kinds are:

- `css`, embedded as `inline-style`;
- `image`, embedded as `data-url`.

## Reserved Region Metadata

A future manifest version may add an optional `regions` field for provenance and
diff tooling. Region metadata is not emitted today.

If added later, region hashes will be provenance aids, not independent
signatures. The authoritative v1 security claim remains the whole generated page
signature produced by `attest` / `attested-html`.
