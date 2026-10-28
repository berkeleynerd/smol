# smol

`smol` is the reference generator for the Attested HTML Static No-JS Profile.
It builds minimal, self-contained XHTML5 pages that are ready to be signed
byte-for-byte with `attest` / `attested-html`. It can also emit flat Gemini
capsules from Gemtext or Markdown source.

## What it does

- Creates small static sites with pages, posts, partials, assets, and theme config.
- Emits XHTML5 as the default and only HTML format, with inline CSS.
- Rejects malformed XML, incompatible HTML structure, and unsupported static markup.
- Emits flat Gemini capsules from `gemtext-v1` and `markdown-xhtml-v1` pages.
- Embeds supported local images as `data:` URLs through `{{image}}` and Markdown images.
- Renders constrained Markdown/XHTML and Gemtext source bodies for HTML output.
- Checks already-built HTML/XHTML/Gemini output against the static no-JS profile.
- Provides a `SMOL ATTESTED MANIFEST V1` comment to both starter themes inside
  the signed payload, with resource provenance and region hashes for drift checks.
  Custom themes may omit it; authorship verification needs no manifest.
- Automatically calls `attest` / `attested-html sign` as the final build step
  when a signing key is configured and a signer is discoverable.

## What it does not do

- No JavaScript.
- No broad CommonMark/GFM in v0; only constrained `markdown-xhtml-v1` and `gemtext-v1` source formats.
- No plugins, arbitrary hooks, or client-side routing. The HTML structure check
  uses `golang.org/x/net/html`; CSS tokenization uses `github.com/tdewolff/parse/v2/css`.
  Signing remains in the separate `attest` tool.
- No external stylesheets, scripts, fonts, iframes, forms, embeds, or remote image fetching.
- No Publii theme rendering and no Handlebars support.
- No cryptographic signing implementation inside `smol`.
- Targets XHTML5, with well-formed XML and HTML-compatible syntax. Historical
  XHTML 1.1 DTDs and external entity declarations are unsupported.

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
smol publish --dry-run
smol publish --sign
smol build --sign
smol build --sign-key FINGERPRINT
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
    index.html
    about.md
    sample/
      index.md
      assets/
  posts/
    hello-world.md
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
`output_mode`, `routing`, `sign_key`, `nav`, and `publish`. If `--sign-key` is supplied,
it overrides `sign_key` in `smol.json`. Use `--sign` to choose a local GPG
secret key interactively for a single build or publish run. Use `--unsigned` to
suppress signing even when a key is configured or selected. The signing key
value is a fingerprint or key ID, not a secret, but private signing keys should
not be placed in cloud workspaces.

## XHTML5 output

`output_mode` defaults to `"xhtml5"`, the only HTML output format. `routing`
selects `"directory"` (default: `/about/index.html`) or `"flat"` (`/about.html`);
posts use `/posts/<slug>/index.html` or `/posts/<slug>.html`. Both choices use
identical XHTML5 and static-resource validation. Gemini is a separate format.
An `index` post keeps its own post route; only the `index` page is the homepage.
For an older flat site, replace `"output_mode": "flat-xhtml-v1"` with
`"output_mode": "xhtml5", "routing": "flat"`.

Every generated page must be well-formed UTF-8 XML with `<!DOCTYPE html>`, an
`html` root in `http://www.w3.org/1999/xhtml`, one `head` with a `title`, and one
`body`. XML and HTML parsing must agree on structure and text; static content
models are checked as well. Exactly one UTF-8 encoding declaration, normally
`<meta charset="utf-8" />`, must use literal metadata (no character references)
and fit entirely within the first 1024 bytes.
Ordinary elements need explicit closing tags;
void elements use `<img … />`, `<br />`, etc. Attributes must be quoted, and
boolean attributes need values, such as `open="open"`. Use Unicode, numeric
references, or XML's predefined entities (`&amp;`, `&lt;`, `&gt;`, `&quot;`,
`&apos;`), rather than HTML-only named entities such as `&nbsp;`.

The inline stylesheet is wrapped in CSS-commented CDATA boundaries, preserving
characters such as `&` and `<` under either XML or HTML parsing. XML declarations,
processing instructions, external DTDs, and entity declarations are rejected.
The final signature is an XML-safe comment after the root element. Both the
unsigned document and the signer's final output are validated in staging;
validation or signing failure preserves existing output.

CSS resource checks use tokenization after CSS input preprocessing and identifier
escape decoding. Quoted comment delimiters cannot hide live declarations.
The same policy applies to stylesheets, decoded `style` attributes, and SVG
attribute values; comments and harmless resource words inside strings are inert.

## Content

Pages and posts are source files under `content/pages/` and `content/posts/`.
The filename is the slug, and the extension selects the source format:

```text
content/pages/index.html
content/pages/about.md
content/posts/hello-world.md
```

Extensions are matched case-insensitively, so `about.MD` is accepted. Slugs
still use the filename before the extension and must match smol's lowercase slug
rule, so `About.MD` is rejected as slug `About`.

Local images require bundle form so assets have a page-local directory:

```text
content/pages/sample/
  index.md
  assets/hero.png
```

A bundle may contain exactly one `index.html`, `index.md`, or `index.gmi`
source file, matched case-insensitively. Other root files with source extensions
inside the bundle are rejected. Non-source files and subdirectories are treated
as local assets; nested `child/index.md` files are not discovered as pages.

Source files may begin with small front matter:

```md
---
title: Hello World
summary: A first signed page.
published: 2026-06-07T00:00:00Z
updated: 2026-06-07T00:00:00Z
tags: [attestation]
draft: false
links:
  up: essays
  previous: kore
  next: lament
  related: [parable-of-old-stone]
---
Write your page here.
```

Front matter is intentionally small and is not YAML. After the closing fence,
the body is read byte-for-byte; no blank line is stripped or added. The accepted
grammar is:

| Construct | Accepted syntax |
| --- | --- |
| Leading BOM | One leading UTF-8 BOM is stripped before parsing or rendering. |
| Opening fence | First non-BOM bytes must be `---` with optional trailing spaces or tabs, followed by LF, CRLF, or EOF. EOF form is treated as unclosed front matter. |
| Closing fence | Line is `---` or `...` with optional trailing spaces or tabs, followed by LF, CRLF, or EOF. |
| Leading blank before fence | Not front matter; the entire file is body content. |
| Line endings | LF and CRLF are accepted. CR-only front matter is rejected. |
| Quoted keys | Surrounding matched `'...'` or `"..."` is stripped before key recognition; keys may not contain whitespace. |
| Unknown top-level keys | Ignored, including inline values, indented child blocks, key-indent list items, and nested maps. |
| Recognized nested under unknown | Top-level smol keys nested under unknown metadata are rejected so safety fields such as `draft` are not silently ignored. |
| Scalars | Bare strings, double-quoted JSON strings, and single-quoted strings. |
| Bare scalars starting with `'` | Parsed as single-quoted and must close; use double quotes or doubled apostrophes for leading apostrophe text. |
| Single quotes | `''` becomes `'`; backslashes are literal; trailing text after the closing quote is rejected. |
| Lists | Inline `[a, "b, c", 'd']` or block lists at/deeper than the key indentation. Quotes delimit inline items only when they start an item, so apostrophes inside bare items are literal. Empty inline list `[]` is valid; bare `tags:` with no items is rejected. Top-level block lists are flat. The first non-list line ends a block list. |
| Links | `links` is a strict block map with `up`, `previous`, `next`, and `related`; those subkeys are not top-level keys and are not guarded inside unknown foreign blocks. |
| Navigation label | Optional `nav_label` scalar names the page in other pages' generated navigation (`Up`/`Previous`/`Next`/`Related` links); when absent, the page title is used. The page's own `<h1>` and `<title>` always use `title`. |
| Dates | RFC3339 timestamps are accepted unchanged. Bare `YYYY-MM-DD` dates are normalized to midnight UTC. Empty date fields are treated as omitted. Other date formats are rejected. |
| Table of contents | `toc: true` generates a table of contents from the page's level-2 Markdown headings, ATX or setext. Anchors are auto-derived (lowercase letters and digits, other runs become single hyphens; non-ASCII characters are dropped); an explicit `{#id}` on an ATX or setext heading overrides. Link and image markup in a collected heading contributes only its label to the entry text and anchor. With `toc: true` every heading anchor on the page must be unique and must not collide with footnote checkbox ids. Duplicate or underivable anchors are errors, as is `toc: true` with no level-2 headings, a non-Markdown body, or `flat-gemini-v1` output. |
| Comments | Unsupported; `#` is literal text. |
| Booleans | `true` and `false` are accepted case-insensitively. |

Recognized keys must be top-level. Duplicate recognized keys and malformed
recognized values are errors. `published`/`updated` are aliases for the internal
`published_utc`/`updated_utc` fields; do not specify both forms for the same
field.

If `title` is absent, `index` becomes `Home` and other slugs are humanized, for
example `hello-world` becomes `Hello World`. The first Markdown `# Heading` is
not inspected. Use explicit `title` front matter when exact casing matters, such
as acronyms or proper nouns.

Link values are page slugs only. Post slugs, draft pages, unknown slugs,
self-links, duplicate `related` targets, and external URLs are rejected. Themes
can render the resolved links with `.PageNav`; the starter themes render them as
generated navigation chrome outside the authored content body. Navigation
labels use the target page's `nav_label` when set, otherwise its title. An
`up: index` link is validated but rendered only as the `Home` link — `Up`
appears in the chrome only when the parent is a page other than home. This is
structural isolation only: signed HTML still covers the full generated page.
Manifest region hashes are provenance and drift-check aids; tamper resistance
comes from the whole-page attested signature.
See `docs/smol-static-nojs-v1.md` for the output profile contract and
`docs/smol-attested-manifest-v1.md` for manifest fields.

A generated table of contents is opt-in per page. With `toc: true`, smol
collects the page's level-2 Markdown headings (ATX or setext) in document
order, injects the derived (or explicit `{#id}`) anchors into the rendered
headings, and exposes the entries to themes as `.Page.TOC`; the classic XHTML
starter renders them as chrome above the authored content:

```md
---
title: Sample
toc: true
---

## First Section

## Custom Anchor {#custom}
```

HTML source files use `.html` and contain an HTML fragment. They may use only
the fixed body functions:

```html
<p>Hello.</p>
{{image "assets/hero.png" "Alt text"}}
<p>{{date .Page.PublishedUTC "2006-01-02"}}</p>
```

Supported image files are PNG, JPG, JPEG, GIF, WebP, and AVIF. SVG image files are
not embedded; constrained inline SVG may be authored in `.html` fragments.
Images are read only from the bundle directory and embedded as
data URLs. Flat files do not have a local asset directory, so `{{image}}` and
Markdown images require bundle form.

Markdown source files use `.md` and render as `markdown-xhtml-v1`. It is a
constrained Markdown/XHTML dialect, not full CommonMark or GFM. Normal text is
HTML-escaped. Supported Markdown includes paragraphs, ATX headings, Setext
headings, inline links, local embedded images, emphasis, strong text, inline
code, ordered lists, unordered lists, static task-list items, blockquotes,
simple pipe tables, fenced code blocks, horizontal rules, and literal numeric
inline notes.

Markdown bodies must be **pure Markdown with a YAML front-matter header** — no
embedded HTML in any form. Two kinds of HTML are rejected at build time, each
with a file and a 1-based line number:

- **HTML elements.** Block and inline tags, closing tags, and comments
  (`<div>`, `</p>`, `<!-- … -->`) are rejected; use the `.html` body format for
  full-HTML authoring, or a code span for literal tag text. Autolink syntax
  (`<https://example.org>`) is likewise rejected; write
  `[label](https://example.org)` instead. A `<` is flagged only when it reads
  as HTML: `<` or `</` followed by a letter, or `<!` beginning a comment or
  declaration. Anything else — `a < b`, `1<2`, `</3`, `<!?`, a trailing `<` —
  is ordinary text, but `a<b` is rejected because `<b` reads as a tag.
- **HTML character entities.** Named, decimal, and hexadecimal references
  (`&nbsp;`, `&amp;`, `&#160;`, `&#xA0;`) are rejected; type the character
  directly — a real non-breaking space, `&`, `—`, and so on. A bare `&` that is
  not a complete `&entity;` reference (e.g. `Tom & Jerry`, a `?a=1&b=2` query
  string) is ordinary text.

Both checks ignore content inside fenced code blocks and inline code spans, so
literal tags and entities can still be displayed as code. Standard CommonMark
backslash escapes (`\*`, `1\.`) are Markdown rather than HTML and are permitted.

Task-list items render as static list text with checkbox glyphs, not form
controls or JavaScript. Markdown links may use relative URLs, fragments,
`http://`, `https://`, `gemini://`, and `mailto:`. `javascript:`, `data:`, and
unsupported URL schemes are rejected.

Markdown images use this form:

```md
![Alt text](assets/hero.png "Optional title")
```

Markdown images must be local files under the page or post content directory,
matching the `{{image}}` content lookup policy. Use bundle form when a Markdown
file references a local image. Remote image URLs, protocol
relative URLs, absolute paths, path traversal, missing files, unsupported
extensions, and SVG are rejected. Supported image types are PNG, JPG, JPEG, GIF,
WebP, and AVIF. Images are embedded as `data:` URLs in the generated XHTML. In
HTML output modes that emit a `SMOL ATTESTED MANIFEST V1` comment, Markdown
image resources are included in that manifest.

The `classic-xhtml` starter stylesheet covers links, images, emphasis, strong
text, inline code, code blocks, ordered lists, unordered lists, task lists,
blockquotes, tables, horizontal rules, and headings with small static CSS rules.

Gemtext source files use `.gmi` and render as `gemtext-v1`. In HTML output modes it
renders Gemtext line types to constrained XHTML. In `flat-gemini-v1` output mode
it emits Gemini capsule pages directly. Supported Gemtext includes text lines,
blank lines, `#`/`##`/`###` headings, `=>` links, `* ` list items, `>` quotes,
and fenced preformatted blocks.

### Source And Output Formats

| Source file | Source format | HTML/XHTML output | `flat-gemini-v1` output |
| --- | --- | --- | --- |
| `.html` | `html` | Rendered as constrained HTML through the selected theme. | Not supported. |
| `.md` | `markdown-xhtml-v1` | Rendered as the supported constrained Markdown/XHTML subset. Markdown images embed as local `data:` URLs. | Rendered lossily to Gemtext. Formatting is reduced to readable text, links become separate `=> URL label` lines, tables become preformatted pipe tables, and Markdown images are rejected. |
| `.gmi` | `gemtext-v1` | Rendered as constrained XHTML from supported Gemtext line types. | Near-native pass-through with a generated page title heading. |

Markdown-to-Gemini conversion preserves document shape where Gemtext has a
matching concept: headings, paragraphs, links, lists, task-list text,
blockquotes, tables as preformatted blocks, fenced code blocks, and horizontal
rules.
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

Capsule output supports pages only. Pages may use `.gmi` or `.md` sources.
`.html` pages and posts are rejected. `smol` writes the page title as the first
`#` heading, then appends the rendered Gemtext body. The generated capsule
writes `index.gmi` for the `index` page and `<slug>.gmi` for other pages.
Themes, templates, CSS, HTML bodies, posts, Markdown images, and signing are not
used by `flat-gemini-v1`.

## Checking Output

`smol check` validates already-built output files or directories against the
static no-JS profile:

```sh
smol check public
smol check --mode xhtml5 public
smol check --mode flat-gemini-v1 public
```

The default `xhtml5` mode checks `.html`, `.htm`, and `.xhtml` files using the
same XML, HTML structure, and static-resource policy for both route layouts.
Use `--mode flat-gemini-v1` for Gemini. `.gmi` files are validated as Gemtext.
`smol check` also validates manifest syntax and declared region hashes when a
manifest is present. It does not verify cryptographic signatures.

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

Without explicit SSH settings, publishing uses your normal SSH configuration,
keys, agent, and `known_hosts`. Optional `publish.identity_file`,
`publish.known_hosts_file`, and `publish.host_key_fingerprint` fields select an
identity and a pinned server key. The known-host file and SHA256 fingerprint
must be supplied together; smol checks their agreement before building or
connecting, freezes the validated public key for the transfer, and uses strict
host-key checking. Explicit SSH settings require batch public-key authentication.
Local paths are relative to the site directory; `~/` expands to the user's home.
The corresponding CLI overrides are `--identity-file`, `--known-hosts-file`,
and `--host-key-fingerprint`. Private key contents do not belong in `smol.json`.

```sh
smol publish
smol publish --out public .
smol publish --no-build
smol publish --sign
smol publish --sign-key FINGERPRINT
smol publish --dry-run
smol publish --check-ssh
smol publish --mode in-place
smol publish --host example.org --user deploy --port 2222 --path /var/www/example
```

HTML publication requires a full expected signing-key fingerprint in
`smol.json.sign_key` or `--sign-key`. `--sign` chooses a local key for the build.
Unsigned HTML remains available for local previews; `--unsigned` cannot publish
HTML. `--no-build` verifies existing pages against the configured fingerprint;
it cannot run signing or accept `--sign` / `--sign-key`.

The default publish builds finalized pages, makes a private output snapshot,
and validates every page's XML, static-resource policy, and embedded OpenPGP
signature with `attest verify --trusted FINGERPRINT`. Each gzip sibling must
decompress to its matching signed page. Symlinks and special files are rejected.
Only that verified snapshot is uploaded. The remote host requires exactly the
expected files and directories, including empty directories, rejects symlinks and
special files, and checks SHA-256 hashes before activating the output, using `sha256sum`,
OpenBSD `sha256`, or `shasum`. Extra files or directories fail the check.
A failed check leaves the live site in place.

The default `publish.mode` is `"swap"`: the destination directory is replaced,
which requires write access to its parent. Use `"mode": "in-place"` or
`--mode in-place` when only the existing web root is writable. That mode stages
inside the web root, applies the same exact inventory/hash gate to the upload,
then replaces files individually by rename. It never renames or chmods the web
root or writes to its parent, and retains existing remote files absent from the
verified snapshot. Replacements are individually atomic; the entire update is
not, so readers can briefly see old and new pages together. Failed transfers or
verification leave existing pages in place; an installation failure can leave a
partially updated site. Unique staging cleanup is attempted on failure.

`--check-ssh` checks existing output paths, SSH access, and destination permissions
without building, uploading, or creating staging directories. It does not certify
signatures or perform a deployment, and cannot be combined with `--dry-run` or
active signing flags.

`--attest PATH` selects the signer/verifier. Gemini publication retains its
separate unsigned behavior.

`--dry-run` previews commands without building, verifying signatures, or
connecting to the host; it does not certify an output directory as publishable.

## Signing

### Signing requirements

Unsigned builds do not require `attest`, `attested-html`, `gpg`, or a private
key. If no signing key is configured, supplied with `--sign-key`, or selected
with `--sign`, `smol` builds normal unsigned HTML even when a signer and local
GPG secret keys are available. The local keyring is inspected only when
interactive `--sign` is requested.

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
configured, supplied, or selected with `--sign`, or when `--unsigned` is
passed. `--unsigned` wins over `smol.json.sign_key`, `--sign-key`, and
interactive signing, which is useful for CI or local preview builds where
signing should be skipped (these builds cannot be published as HTML):

```sh
smol build
smol build --unsigned
```

### Build signed

Signed builds call `attest` / `attested-html` for each generated page when
`--sign-key` or `smol.json.sign_key` is non-empty, or when `--sign` is used and
a key is selected. If a signing key is active but no signer can be resolved, the
build fails before rendering pages:

Gemini capsule output does not support signing yet. If `flat-gemini-v1` is
configured with a signing key, the build fails unless `--unsigned` is passed.

```sh
smol build --sign
smol build --sign-key FINGERPRINT
smol build --sign-key FINGERPRINT --attest /path/to/attested-html
```

### Precompressed siblings

Every generated HTML page is also written as a deterministic
best-compression gzip sibling (`index.html.gz`) of its final bytes — for
signed builds, the bytes after the attestation trailer is appended — so
static servers configured for precompressed content (OpenBSD httpd
`gzip-static`, nginx `gzip_static on;`) can serve the smaller file while
clients decompress to exactly the attested page. `smol check` ignores the
`.gz` siblings; `smol publish` verifies that they match the signed pages, and `flat-gemini-v1` capsules do not emit them.

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

It validates the final signed document without rewriting its bytes. It does
not duplicate signing logic or modify output after the signer writes it. The final filenames stay as normal `.html` paths; `smol`
does not emit separate `.attested.html` files.

Signed HTML is byte-sensitive. Git line-ending normalization can break
byte-for-byte signatures, so generated sites include:

```gitattributes
public/**/*.html -text
build/**/*.html -text
*.attested.html -text
```
