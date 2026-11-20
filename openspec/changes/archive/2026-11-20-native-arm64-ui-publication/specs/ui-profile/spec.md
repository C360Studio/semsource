## MODIFIED Requirements

### Requirement: Workbench and headless release evidence

SemSource SHALL maintain independent release evidence for the headless core and the SemSource-owned
`ui` profile. The UI smoke SHALL actively verify the shell, SemSource health/source status, query
reachability, accessible result/detail navigation, and the advertised graph state against a real
SemSource backend. The headless smoke SHALL prove the same backend starts and serves its non-UI
contracts without any UI artifact.

Pull requests SHALL run locked UI quality/browser gates, clean production-image verification, and
release-verifier contract tests without publishing. Only trusted pushes to `main` or a valid
`v`-prefixed SemVer tag SHALL receive package-write permission and publish the SemSource-owned image
to `ghcr.io/c360studio/semsource-ui` for `linux/amd64` and `linux/arm64`. Each platform SHALL be
built on a runner of that architecture, never under emulation, and pushed by digest without a tag; a
separate merge step SHALL tag the multi-platform index only when exactly one valid digest per required
platform is present, and SHALL report the index digest read back from the registry. Re-running a
failed platform build SHALL replace only that platform's digest.

A `main` publication SHALL emit `latest` and `sha-<full-revision>` tags, with the SHA tag as the
verification tag and OCI version. A release publication SHALL emit the exact `v<semver>` tag and the
plain `<semver>` tag, with the `v` tag as the verification tag and the plain SemVer as the OCI version.
Every platform image SHALL carry the derived `org.opencontainers.image.version` and the full Git
revision in `org.opencontainers.image.revision`. `latest` SHALL NOT qualify as release evidence even
when digest-qualified.

The publish job SHALL expose repository, manifest digest, verification tag, version, and revision to
a separate release-smoke job. That job SHALL verify that the tag currently resolves to the supplied
digest, the exact digest is a multi-platform OCI index containing both required platforms, each
platform image carries the expected OCI labels, and the locally pulled exact reference reports the
same repository digest. It SHALL run the released profile with that exact tag-plus-digest and verify
the Compose-rendered image and running container pin. Success evidence SHALL record the authoritative
GitHub Actions run URL and attempt; failure SHALL preserve actionable profile diagnostics.

#### Scenario: Pinned workbench compatibility is tested

- **GIVEN** the released `ui` profile is started with its immutable SemSource artifact
- **WHEN** the SemSource workbench smoke runs
- **THEN** it verifies the workbench shell, authoritative readiness, source status, search or query
  reachability, accessible search result/detail selection, and the advertised graph state

#### Scenario: Pull request validates without publishing

- **WHEN** a pull request runs CI
- **THEN** UI quality, browser, clean-image, and release-verifier contract gates run
- **AND** no UI image is published and the job has no package-write permission

#### Scenario: Trusted main publication is derived deterministically

- **GIVEN** a trusted push to `main` at a full Git revision
- **WHEN** the UI image is published
- **THEN** it receives `latest` and `sha-<full-revision>` tags
- **AND** the SHA tag, not `latest`, is passed to release verification
- **AND** OCI version is `sha-<full-revision>` and OCI revision is the full revision

#### Scenario: Trusted release publication is derived deterministically

- **GIVEN** a trusted push of a valid `v`-prefixed SemVer tag
- **WHEN** the UI image is published
- **THEN** it receives both the exact `v<semver>` and plain `<semver>` tags
- **AND** the exact `v` tag is passed to release verification
- **AND** OCI version is the plain SemVer and OCI revision is the full revision

#### Scenario: Published manifest becomes release evidence

- **GIVEN** the publish job reports an image repository, verification tag, manifest digest, version,
  and full revision
- **WHEN** the separate release-smoke job verifies the publication
- **THEN** the tag resolves to that exact multi-platform manifest digest
- **AND** required platform labels and the locally observed repository digest match
- **AND** Compose rendering, the running container, and `task ui:smoke` use the same immutable pin
- **AND** success evidence records the run URL and attempt

#### Scenario: Published-image verification fails

- **WHEN** registry provenance, manifest platforms, OCI labels, local repository digest, Compose pin,
  running-container pin, or released-profile smoke does not match
- **THEN** release evidence is not published as successful
- **AND** the CI run retains actionable failure diagnostics

#### Scenario: Headless release path is tested independently

- **WHEN** the SemSource core smoke runs without optional UI profiles
- **THEN** it proves backend readiness and HTTP/MCP/query availability without pulling or starting a UI
  artifact

#### Scenario: Platform images build natively and merge both-or-nothing

- **GIVEN** a trusted push to `main` or a valid release tag
- **WHEN** the UI image is published
- **THEN** `linux/amd64` and `linux/arm64` each build on a native runner without QEMU and push by digest
- **AND** the tagged index is created only from exactly one valid digest per platform
- **AND** a missing, malformed, or extra digest, or a tag outside the repository, publishes nothing
