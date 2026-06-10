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
  "resources": [],
  "regions": [
    {
      "name": "authored-content",
      "sha256": "..."
    }
  ]
}
```

`profile` uses the canonical slash-form identifier `smol/static-nojs-v1`.
`regions` is optional and is omitted when the rendered HTML has no recognized
region markers. When a nested HTML file has recognized region markers, `smol`
emits matching `regions` entries.

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

`sha256` is the lowercase 64-character hexadecimal SHA-256 digest of the
embedded resource bytes.

## Regions

`regions` records byte-exact hashes for recognized structural regions present in
the rendered HTML. Region entries use:

```json
{
  "name": "authored-content",
  "sha256": "..."
}
```

Supported current region names are:

- `authored-content`, for the inner bytes of
  `<!--smol:authored-content-->...<!--/smol:authored-content-->`;
- `generated-navigation`, for the inner bytes of
  `<!--smol:generated-navigation-->...<!--/smol:generated-navigation-->`.

Recognized v1 regions are identified by those exact literal comment marker
strings:

```html
<!--smol:authored-content-->
<!--/smol:authored-content-->
<!--smol:generated-navigation-->
<!--/smol:generated-navigation-->
```

The hash covers the raw byte string after the single exact open marker and
before the first exact close marker after it. It is whitespace- and
line-ending-sensitive; region hashing does not normalize line endings. The
`main#content` element in shipped themes is a semantic landmark and link
target, not a region boundary.

For known region names, `sha256` is the lowercase 64-character hexadecimal
SHA-256 digest of the region inner bytes. Manifest readers should validate the
shape of unknown future region names but only hash-verify region names they
understand. Unknown future region names may use a non-hex `sha256` string in v1;
digest agility with an explicit algorithm field is deferred.

Region hashes are provenance and drift-check aids, not independent signatures
and not a tamper-resistance claim. The authoritative v1 security claim remains
the whole generated page signature produced by `attest` / `attested-html`.

The manifest must remain outside hashed regions. `smol build` renders nested
HTML manifests in two passes and rejects pages whose final region hashes differ
from the hashes computed before the region metadata was embedded.
