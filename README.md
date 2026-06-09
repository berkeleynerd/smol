# smol

`smol` is the reference generator for the Attested HTML Static No-JS Profile.
It builds minimal, self-contained static HTML pages that are ready to be signed
byte-for-byte with `attest` / `attested-html`. It can also emit flat Gemini
capsules from Gemtext or Markdown source.

## What it does

- Creates small static sites with pages, posts, partials, assets, and theme config.
- Emits single-file HTML with inline CSS.
- Emits flat Gemini capsules from `gemtext-v1` and `markdown-xhtml-v1` pages.
- Embeds supported local images as `data:` URLs through `{{image}}` and Markdown images.
- Renders constrained Markdown/XHTML and Gemtext source bodies for HTML output.
- Checks already-built HTML/XHTML/Gemini output against the static no-JS profile.
- Adds a `SMOL ATTESTED MANIFEST V1` comment to nested HTML output inside the
  signed payload.
- Automatically calls `attest` / `attested-html sign` as the final build step
  when a signing key is configured and a signer is discoverable.

## What it does not do

- No JavaScript.
- No broad CommonMark/GFM in v0; only constrained `markdown-xhtml-v1` and `gemtext-v1` source formats.
- No third-party packages, plugins, arbitrary hooks, or client-side routing.
- No external stylesheets, scripts, fonts, iframes, forms, embeds, or remote image fetching.
- No Publii theme rendering and no Handlebars support.
- No cryptographic signing implementation inside `smol`.
- No strict historical XHTML DTD conformance; XHTML-style output is used for
  consistent static markup and closing discipline, not DTD purity.

## Install

Build from source:

```sh
go build -o smol .
```

Run tests:

```sh
go test ./...
```

## Usage

```sh
smol init mysite
cd mysite
smol new post hello-world "Hello World"
smol build
smol check public
smol build --sign-key FINGERPRINT
smol publish --dry-run
smol publish
```

Build options must appear before positional arguments:

```sh
smol build --out public .
```

For the quickest Markdown/XHTML publishing demo, start with the classic XHTML
starter:

```sh
smol init --starter classic-xhtml mysite
cd mysite
smol build
```

The `classic-xhtml` starter creates a flat XHTML site with `.md` bodies, a
classic typographic stylesheet, and a sample page that exercises the supported
Markdown elements.

## Site Structure

```text
smol.json
content/
  pages/
    index/
      page.json
      body.html
  posts/
themes/
  default/
    theme.json
    templates/
    partials/
    assets/
public/
.gitattributes
```

## Minimal `smol.json`

```json
{
  "format": "smol-site-v1",
  "title": "Example Site",
  "base_url": "https://example.org"
}
```

Optional fields include `description`, `language`, `publisher`, `theme`,
`output_mode`, `sign_key`, `nav`, and `publish`. If `--sign-key` is supplied,
it overrides `sign_key` in `smol.json`. Use `--unsigned` to suppress signing
even when a key is configured. The signing key value is a fingerprint or key ID,
not a secret, but private signing keys should not be placed in cloud workspaces.

## Content

Each page or post has `page.json` and `body.html` by default:

```json
{
  "format": "smol-page-v1",
  "kind": "post",
  "title": "Hello World",
  "slug": "hello-world",
  "summary": "A first signed page.",
  "published_utc": "2026-06-07T00:00:00Z",
  "updated_utc": "2026-06-07T00:00:00Z",
  "tags": ["attestation"],
  "draft": false
}
```

Pages may declare explicit page-navigation links:

```json
"links": {
  "up": "essays",
  "previous": "kore",
  "next": "lament",
  "related": ["parable-of-old-stone"]
}
```

Link values are page slugs only. Post slugs, draft pages, unknown slugs,
self-links, duplicate `related` targets, and external URLs are rejected. Themes
can render the resolved links with `.PageNav`; the starter themes render them as
generated navigation chrome outside the authored content body. This is
structural isolation only: signed HTML still covers the full generated page.
See `docs/smol-static-nojs-v1.md` for the output profile contract and
`docs/smol-attested-manifest-v1.md` for manifest fields.

`body.html` is an HTML fragment. It may use only the fixed body functions:

```html
<p>Hello.</p>
{{image "assets/hero.png" "Alt text"}}
<p>{{date .Page.PublishedUTC "2006-01-02"}}</p>
```

Supported image types are PNG, JPG, JPEG, GIF, WebP, and AVIF. SVG is not
supported in v0. Images are read only from the content directory and embedded as
data URLs.

The optional `markdown-xhtml-v1` body format reads `body.md`. It is a
constrained Markdown/XHTML dialect, not full CommonMark or GFM. Normal text is
HTML-escaped. Supported Markdown includes paragraphs, ATX headings, Setext
headings, inline links, local embedded images, emphasis, strong text, inline
code, ordered lists, unordered lists, static task-list items, blockquotes,
simple pipe tables, fenced code blocks, horizontal rules, raw allowlisted XHTML
blocks, and literal numeric inline notes.

Task-list items render as static list text with checkbox glyphs, not form
controls or JavaScript. Markdown links may use relative URLs, fragments,
`http://`, `https://`, `gemini://`, and `mailto:`. `javascript:`, `data:`, and
unsupported URL schemes are rejected.

Markdown images use this form:

```md
![Alt text](assets/hero.png "Optional title")
```

Markdown images must be local files under the page or post content directory,
matching the `{{image}}` content lookup policy. Remote image URLs, protocol
relative URLs, absolute paths, path traversal, missing files, unsupported
extensions, and SVG are rejected. Supported image types are PNG, JPG, JPEG, GIF,
WebP, and AVIF. Images are embedded as `data:` URLs in the generated XHTML. In
HTML output modes that emit a `SMOL ATTESTED MANIFEST V1` comment, Markdown
image resources are included in that manifest.

The `classic-xhtml` starter stylesheet covers links, images, emphasis, strong
text, inline code, code blocks, ordered lists, unordered lists, task lists,
blockquotes, tables, horizontal rules, and headings with small static CSS rules.

The optional `gemtext-v1` body format reads `body.gmi`. In HTML output modes it
renders Gemtext line types to constrained XHTML. In `flat-gemini-v1` output mode
it emits Gemini capsule pages directly. Supported Gemtext includes text lines,
blank lines, `#`/`##`/`###` headings, `=>` links, `* ` list items, `>` quotes,
and fenced preformatted blocks.

### Source and output formats

| Source body format | HTML/XHTML output | `flat-gemini-v1` output |
| --- | --- | --- |
| `html` / `body.html` | Rendered as constrained HTML through the selected theme. | Not supported. |
| `markdown-xhtml-v1` / `body.md` | Rendered as the supported constrained Markdown/XHTML subset. Markdown images embed as local `data:` URLs. | Rendered lossily to Gemtext. Formatting is reduced to readable text, links become separate `=> URL label` lines, tables become preformatted pipe tables, and Markdown images are rejected. |
| `gemtext-v1` / `body.gmi` | Rendered as constrained XHTML from supported Gemtext line types. | Near-native pass-through with a generated page title heading. |

Markdown-to-Gemini conversion preserves document shape where Gemtext has a
matching concept: headings, paragraphs, links, lists, task-list text,
blockquotes, tables as preformatted blocks, fenced code blocks, horizontal
rules, and raw XHTML text extraction from valid allowlisted XML fragments.
Emphasis, strong text, inline code, and footnote markup are stripped to readable
plain text. Markdown images are rejected for `flat-gemini-v1`; binary image
asset copying into capsules is intentionally not implemented.

`published_utc` and `updated_utc` are publisher claims, not trusted timestamps.
Trusted timestamping belongs in the external attestation evidence, not in page
metadata.

## Gemini Capsules

Set `output_mode` to `flat-gemini-v1` to generate a flat Gemini capsule:

```json
{
  "format": "smol-site-v1",
  "title": "Example Capsule",
  "base_url": "gemini://example.org",
  "output_mode": "flat-gemini-v1",
  "sign_key": ""
}
```

Capsule output supports pages only. Pages may use `body_format: "gemtext-v1"`
with `body.gmi` or `body_format: "markdown-xhtml-v1"` with `body.md`. HTML body
pages and posts are rejected. `smol` writes the page title as the first `#`
heading, then appends the rendered Gemtext body. The generated capsule writes
`index.gmi` for the `index` page and `<slug>.gmi` for other pages. Themes,
templates, CSS, HTML bodies, posts, Markdown images, and signing are not used by
`flat-gemini-v1`.

## Checking Output

`smol check` validates already-built output files or directories against the
static no-JS profile:

```sh
smol check public
smol check --mode flat-xhtml-v1 public
smol check --mode flat-gemini-v1 public
```

The default mode applies the strict nested HTML policy to `.html` and `.xhtml`
files. Use `--mode flat-xhtml-v1` for flat XHTML output and
`--mode flat-gemini-v1` for Gemini capsule output. `.gmi` files are validated as
Gemtext. `smol check` validates static policy, inline CSS, and manifest
well-formedness when a manifest is present; it does not verify cryptographic
signatures.

## Publishing

`smol publish` builds the site, then publishes the output directory over SSH
using `scp` and remote `ssh` commands. Configure the target in `smol.json`:

```json
{
  "format": "smol-site-v1",
  "title": "Example Site",
  "base_url": "https://example.org",
  "publish": {
    "method": "scp",
    "host": "example.org",
    "user": "deploy",
    "port": 22,
    "path": "/var/www/example"
  }
}
```

Publishing uses your normal SSH configuration, keys, agent, and `known_hosts`.
It does not support passwords or secret fields in `smol.json`.

```sh
smol publish
smol publish --out public .
smol publish --no-build
smol publish --dry-run
smol publish --host example.org --user deploy --port 2222 --path /var/www/example
```

By default, `smol publish` performs a clean build of the local output directory,
uploads that directory to a temporary sibling path on the remote host, then
swaps it into place. The remote target is replaced rather than overlaid, so
files removed from the local output disappear remotely. `--no-build` skips the
build and publishes the existing output directory. `--unsigned` applies only to
the build phase.

## Signing

### Signing requirements

Unsigned builds do not require `attest`, `attested-html`, `gpg`, or a private
key. If no signing key is configured, `smol` builds normal unsigned HTML even
when a signer happens to be installed.

Signed builds require:

- A discoverable `attest` or `attested-html` signer.
- A local `gpg` executable.
- A private key in the local GPG secret keyring.

`--sign-key` and `smol.json.sign_key` name a key ID or fingerprint, not a
secret. Private signing keys should not be placed in this repository or in cloud
workspaces. For platform-specific setup, see the attest guides for
[macOS](https://github.com/berkeleynerd/attest/blob/main/docs/setup/macos.md),
[Linux](https://github.com/berkeleynerd/attest/blob/main/docs/setup/linux.md),
[OpenBSD](https://github.com/berkeleynerd/attest/blob/main/docs/setup/openbsd.md),
and [Windows](https://github.com/berkeleynerd/attest/blob/main/docs/setup/windows.md).

### Build unsigned

Unsigned builds write finalized HTML to `public/` when no signing key is
configured, or when `--unsigned` is passed. `--unsigned` wins over both
`smol.json.sign_key` and `--sign-key`, which is useful for CI or local preview
builds where a signing key is configured but signing should be skipped:

```sh
smol build
smol build --unsigned
```

### Build signed

Signed builds call `attest` / `attested-html` for each generated page when
`--sign-key` or `smol.json.sign_key` is non-empty. If a signing key is active
but no signer can be resolved, the build fails before rendering pages:

Gemini capsule output does not support signing yet. If `flat-gemini-v1` is
configured with a signing key, the build fails unless `--unsigned` is passed.

```sh
smol build --sign-key FINGERPRINT
smol build --sign-key FINGERPRINT --attest /path/to/attested-html
```

Explicit `--attest` or `--attested-html` paths win. If no explicit signer path
is supplied, `smol` searches `PATH` for `attest`, then `attested-html`. It then
checks sibling directories beside the resolved `smol` executable: `../attest`
and `../attested-html`. If a sibling is a dependency-free Go project with
`go.mod` and `main.go`, `smol` logs `building signer from ...` and builds a
temporary signer for the current build. Sibling projects with `require`
dependencies in `go.mod` are rejected to avoid unexpected dependency fetches.

`smol` uses this command shape:

```text
SIGNER sign -k SIGNING_KEY -o OUTPUT.html --force INPUT.html
```

It does not duplicate signing logic and does not modify signed output after
the signer writes it. The final filenames stay as normal `.html` paths; `smol`
does not emit separate `.attested.html` files.

Signed HTML is byte-sensitive. Git line-ending normalization can break
byte-for-byte signatures, so generated sites include:

```gitattributes
public/**/*.html -text
build/**/*.html -text
*.attested.html -text
```
