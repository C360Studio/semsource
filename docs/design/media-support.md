# Media Support — Design Document

> **Status:** Draft | **Date:** March 2026
> **Scope:** Image ingestion (Phase 1), Video + keyframe extraction (Phase 2)

---

## Motivation

Early adopters need to ingest visual assets alongside code and documents. Use cases include:

- Architecture diagrams, wireframes, and screenshots stored in repos
- Video walkthroughs and training recordings
- Extracting keyframes from video as standalone image entities
- Future: ML vision processors that label images and connect them to code entities

Images and video keyframes share a common representation: visual content with dimensions, format metadata, and a binary blob stored in ObjectStore. Designing them together ensures consistent semantics even though implementation is phased.

---

## Entity Model

### New Entity Types

| Entity Type | Domain | Description | Phase |
|------------|--------|-------------|-------|
| `image` | `media` | A static image file (png, jpg, svg, webp, gif) | 1 |
| `video` | `media` | A video file (mp4, webm, mov, avi) | 2 |
| `keyframe` | `media` | An image extracted from a video at a specific timestamp | 2 |

### Entity ID Construction

Follows the existing 6-part pattern: `{org}.semsource.{domain}.{system}.{type}.{instance}`

| Entity Type | Construction | Example |
|------------|--------------|---------|
| Image | `org.semsource.media.{repo-slug}.image.{sha256[:6]}` | `acme.semsource.media.github.com-acme-app.image.c3f2a1` |
| Video | `org.semsource.media.{repo-slug}.video.{sha256[:6]}` | `acme.semsource.media.github.com-acme-app.video.b7d9e4` |
| Keyframe | `org.semsource.media.{repo-slug}.keyframe.{video-sha[:4]}-{ts}` | `acme.semsource.media.github.com-acme-app.keyframe.b7d9-15s` |

**Identity rules:**
- Image/video instance = `sha256(file content)[:6]` — intrinsic, deterministic
- Keyframe instance = `{video-sha[:4]}-{timestamp}` — deterministic from video content + position
- Timestamp format in keyframe IDs: seconds from start, e.g., `15s`, `1m30s`
- Same file across independent SemSource instances produces identical IDs (`public.*` compatible)

### System Slug

Same convention as other handlers. For local repo files: repo slug from git remote or directory name. For standalone image paths: directory slug.

---

## Vocabulary

### Standard Vocabulary Alignment

Media predicates map to three established ontologies, following the same pattern used elsewhere in the project (Dublin Core for docs, BFO for structure, PROV-O for provenance):

| Standard | Namespace | Coverage |
|----------|-----------|----------|
| [W3C Media Ontology](https://www.w3.org/TR/mediaont-10/) | `http://www.w3.org/ns/ma-ont#` | Frame size, duration, format, frame rate, compression, bitrate |
| [Schema.org](https://schema.org/ImageObject) | `https://schema.org/` | Content URL, content size, encoding format, thumbnail, width, height |
| [FOAF](https://xmlns.com/foaf/spec/) | `http://xmlns.com/foaf/0.1/` | `depicts` / `depiction` edge semantics |
| Dublin Core (existing) | `http://purl.org/dc/terms/` | `format` (MIME type), `title`, `created` |

### Predicates (`source.media.*`)

Each predicate lists its standard IRI mapping for RDF export.

**Core metadata (Phase 1 — images):**

| Predicate | Type | Standard IRI | Description |
|-----------|------|-------------|-------------|
| `source.media.type` | string | `ma:hasMediaType` | Entity type: "image", "video", "keyframe" |
| `source.media.mime_type` | string | `dc:format` | MIME type: image/png, video/mp4, etc. |
| `source.media.width` | int | `schema:width` | Width in pixels |
| `source.media.height` | int | `schema:height` | Height in pixels |
| `source.media.file_path` | string | (semsource) | File path relative to source root |
| `source.media.file_hash` | string | (semsource) | SHA256 for staleness detection |
| `source.media.file_size` | int | `schema:contentSize` | File size in bytes |
| `source.media.format` | string | `ma:hasFormat` | Decoded format: "png", "jpeg", "svg" |
| `source.media.storage_ref` | string | `schema:contentUrl` | ObjectStore key for binary |
| `source.media.thumbnail_ref` | string | `schema:thumbnail` | ObjectStore key for thumbnail |

**Video metadata (Phase 2):**

| Predicate | Type | Standard IRI | Description |
|-----------|------|-------------|-------------|
| `source.media.duration` | string | `ma:duration` | Duration ("1m30s") |
| `source.media.frame_rate` | float | `ma:frameRate` | Frames per second |
| `source.media.codec` | string | `ma:hasCompression` | Codec: "h264", "vp9", "av1" |
| `source.media.bitrate` | int | `ma:averageBitRate` | Average bitrate in kbps |
| `source.media.keyframe_count` | int | (semsource) | Extracted keyframe count |

**Keyframe metadata (Phase 2):**

| Predicate | Type | Standard IRI | Description |
|-----------|------|-------------|-------------|
| `source.media.timestamp` | string | (semsource) | Position in video ("15s") |
| `source.media.frame_index` | int | (semsource) | Sequential index (1-indexed) |

**ML vision (future, reserved namespace):**

| Predicate | Type | Standard IRI | Description |
|-----------|------|-------------|-------------|
| `source.media.vision.labels` | array | `schema:keywords` | ML-detected labels |
| `source.media.vision.description` | string | `schema:description` | ML-generated description |
| `source.media.vision.confidence` | float | (semsource) | Overall confidence score |
| `source.media.vision.objects` | array | (semsource) | Detected objects with bounding boxes |
| `source.media.vision.text` | string | (semsource) | OCR-extracted text |
| `source.media.vision.model` | string | `prov:wasGeneratedBy` | Model that produced labels |

Vision predicates are reserved but not implemented. A downstream `VisionProcessor` will populate them. The entity structure supports adding these triples without schema changes.

### Edge Types

| Edge | Direction | Standard IRI | Description | Phase |
|------|-----------|-------------|-------------|-------|
| `keyframe_of` | keyframe -> video | `schema:isPartOf` | Links keyframe to source video | 2 |
| `depicts` | image/keyframe -> entity | `foaf:depicts` | Visual representation of a code/doc entity | future |
| `thumbnail_of` | image -> entity | `schema:thumbnail` (inverse) | Preview image | future |

Existing edges reused:
- `contains` (`schema:hasPart`) — video -> keyframes (parent-child)
- `belongs` (`code.structure.belongs`, BFO `part_of`) — keyframe -> video (child-parent)

### Class IRIs

```go
// W3C Media Ontology aligned
MaNamespace = "http://www.w3.org/ns/ma-ont#"

ClassImage    = MaNamespace + "Image"     // ma:Image — static image file
ClassVideo    = MaNamespace + "VideoTrack" // ma:VideoTrack — video file
ClassKeyframe = Namespace + "Keyframe"    // semsource-specific — extracted video frame
```

Schema.org equivalents registered via `owl:equivalentClass`:
- `ClassImage` ↔ `schema:ImageObject`
- `ClassVideo` ↔ `schema:VideoObject`

Extends: `bfo:GenericallyDependentContinuant`, `cco:InformationContentEntity`, `prov:Entity`

---

## Architecture

### Binary Flow

```
[Image/Video File]
       |
  [MediaHandler]
       |
       +---> binary data ---> ObjectStore (via BinaryStorable)
       |                          returns StorageReference
       +---> metadata -----> RawEntity
                                |
                           [Normalizer]
                                |
                           [GraphEvent] ---> WebSocket (JSON, refs only)
```

The graph event is always JSON. It carries `StorageReference` keys, not raw bytes. Consumers fetch binary from ObjectStore on demand.

### ObjectStore Integration

SemSource gains a new dependency: NATS ObjectStore connection. This is required for media sources — without it, there is nowhere to store binary content.

**Config addition:**

```json
{
  "namespace": "acme",
  "object_store": {
    "bucket": "semsource-media",
    "ttl": "720h"
  },
  "sources": [...]
}
```

**Storage key format:**

```
media/image/{entity-id-slug}_{timestamp}
media/video/{entity-id-slug}_{timestamp}
media/keyframe/{entity-id-slug}_{timestamp}
media/thumbnail/{entity-id-slug}_{timestamp}
```

Uses the existing time-bucketed key pattern from semstreams ObjectStore.

**Thumbnail generation:**

For images larger than a configurable threshold (default 512x512), generate a resized thumbnail and store it separately. The thumbnail key goes in `source.media.thumbnail_ref`. This keeps the graph lightweight — consumers can fetch thumbnails for previews without pulling full-resolution images.

---

## Phase 1: Image Handler

### Scope

- Scan configured paths for image files (png, jpg, jpeg, gif, webp, svg)
- Read image metadata (dimensions, format) using Go standard library (`image` package)
- Store binary content in ObjectStore via `BinaryStorable`
- Emit `RawEntity` with media predicates and storage reference
- Watch for changes via fsnotify (same pattern as DocHandler)
- Support RETRACT on file deletion

### Handler Interface

```go
type ImageHandler struct {
    store    objectstore.Store  // semstreams ObjectStore
    watcher  *fswatcher.Watcher
}

func (h *ImageHandler) SourceType() string { return "image" }
func (h *ImageHandler) Supports(entry config.SourceEntry) bool
func (h *ImageHandler) Ingest(ctx context.Context, entry config.SourceEntry) ([]handler.ChangeEvent, error)
func (h *ImageHandler) Watch(ctx context.Context, entry config.SourceEntry, ch chan<- handler.ChangeEvent) error
```

### Supported Formats

| Format | MIME Type | Dimensions | Notes |
|--------|----------|------------|-------|
| PNG | image/png | Go `image` stdlib | Full support |
| JPEG | image/jpeg | Go `image/jpeg` stdlib | Full support |
| GIF | image/gif | Go `image/gif` stdlib | First frame dimensions |
| WebP | image/webp | `golang.org/x/image/webp` | Decode config only |
| SVG | image/svg+xml | Parse viewBox attribute | No rasterization |

### Config Entry

```json
{
  "type": "image",
  "paths": ["assets/", "docs/images/", "screenshots/"],
  "watch": true,
  "extensions": ["png", "jpg", "jpeg", "gif", "webp", "svg"],
  "max_file_size": "50MB",
  "generate_thumbnails": true,
  "thumbnail_max_dimension": 512
}
```

### CLI Wizard

```
Image sources
  Enter paths to scan for images (e.g. assets/, docs/images/).
  (detected: assets/, docs/images/)
  Paths (one per line, empty line to finish):
  >
  Maximum file size [50MB]:
  Generate thumbnails? [Y/n]:
```

---

## Phase 2: Video Handler + Keyframes (Future)

### Scope

- Scan configured paths for video files (mp4, webm, mov, avi)
- Extract metadata: duration, frame rate, codec, dimensions
- Extract keyframes at configurable intervals (default: scene change detection or fixed interval)
- Store video binary + each keyframe image in ObjectStore
- Emit video entity + keyframe entities with parent-child edges
- Watch for changes via fsnotify

### Keyframe Extraction Strategy

**Option A: ffmpeg (recommended)**
- Shell out to `ffmpeg` for keyframe extraction
- Widely available, battle-tested, supports all formats
- System dependency — handler checks for `ffmpeg` in PATH
- `Available()` returns `(false, "ffmpeg not found")` if missing

**Option B: asticode/go-astiav (upgrade path)**
- Modern, actively maintained CGo bindings to FFmpeg libav
- Full programmatic control — decode only I-frames in-process
- Requires FFmpeg dev headers at build time, cross-compilation is painful
- Worth considering if subprocess overhead becomes a bottleneck at scale

**Evaluated and rejected:**
- `giorgisio/goav`, `3d0c/gmf` — abandoned/stale CGo bindings
- `nareix/joy4` — pure Go, but only identifies keyframe packets, cannot decode frames
- `abema/go-mp4` — pure Go MP4 parser, can find sync samples but no video decoding
- `gen2brain/mpeg` — pure Go, MPEG-1 only
- No production-quality pure-Go H.264/H.265 decoder exists

### Keyframe Extraction Modes

| Mode | Description | Config |
|------|-------------|--------|
| `interval` | Fixed time interval (e.g., every 30s) | `"keyframe_mode": "interval", "keyframe_interval": "30s"` |
| `scene` | ffmpeg scene change detection | `"keyframe_mode": "scene", "scene_threshold": 0.3` |
| `iframes` | Extract all I-frames from video stream | `"keyframe_mode": "iframes"` |

Default: `interval` at 30s — simple, predictable, no ffmpeg filter complexity.

### Video Config Entry

```json
{
  "type": "video",
  "paths": ["recordings/", "training/"],
  "watch": true,
  "extensions": ["mp4", "webm", "mov"],
  "max_file_size": "2GB",
  "keyframe_mode": "interval",
  "keyframe_interval": "30s",
  "generate_thumbnails": true
}
```

### Entity Relationships

```
video (acme.semsource.media.repo.video.b7d9e4)
  ├── contains → keyframe (acme.semsource.media.repo.keyframe.b7d9-0s)
  ├── contains → keyframe (acme.semsource.media.repo.keyframe.b7d9-30s)
  ├── contains → keyframe (acme.semsource.media.repo.keyframe.b7d9-1m0s)
  └── contains → keyframe (acme.semsource.media.repo.keyframe.b7d9-1m30s)
```

Each keyframe entity has:
- Its own ObjectStore binary (the extracted frame as JPEG/PNG)
- `source.media.timestamp` — position in source video
- `source.media.frame_index` — sequential index (1, 2, 3...)
- `keyframe_of` edge back to parent video
- `code.structure.belongs` triple linking to parent video

---

## Future: Vision Processor Integration

The `source.media.vision.*` predicate namespace is reserved for downstream ML processors. The pattern follows FederationProcessor — a processor sits in the consumer flow and enriches media entities with vision labels.

```
[SemSource] --media entities--> [WebSocket]
                                    |
[Consumer Flow]                     v
                           [VisionProcessor]
                                    |
                              adds triples:
                              - vision.labels
                              - vision.description
                              - vision.objects
                              - vision.text (OCR)
                                    |
                              [graph ingestion]
```

The VisionProcessor:
1. Receives media entities from the graph event stream
2. Fetches binary from ObjectStore via `StorageReference`
3. Sends to vision model (Claude, OpenAI Vision, local model)
4. Appends `source.media.vision.*` triples to the entity
5. Optionally creates `depicts` edges linking images to code entities

This keeps SemSource focused on ingestion. Vision labeling is a consumer-side concern.

---

## Implementation Plan

### Phase 1 Milestones (Image Support)

1. **Vocabulary**: Add `source.media.*` predicates and class IRIs
2. **Config**: Add `image` source type to `SourceEntry`, add `object_store` config section
3. **Handler**: Implement `ImageHandler` with Ingest/Watch/RETRACT
4. **ObjectStore wiring**: Connect engine to ObjectStore, pass to handler
5. **CLI wizard**: Add image source wizard with path detection
6. **Tests**: Unit tests for handler, vocabulary registration, ID construction

### Phase 2 Milestones (Video + Keyframes)

1. **Handler**: Implement `VideoHandler` with ffmpeg keyframe extraction
2. **Config**: Add `video` source type with keyframe mode settings
3. **CLI wizard**: Add video source wizard with ffmpeg availability check
4. **Tests**: Unit tests, integration tests with sample video files

### Future

- VisionProcessor in semstreams/processor
- `depicts` edge creation from vision labels
- OCR text extraction as searchable triples
- Multimodal embedding support
