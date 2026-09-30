## ADDED Requirements

### Requirement: Component configuration changes explicitly require restart

After successful boot, SemSource SHALL treat changes that add, remove, or reconfigure components as
next-boot desired state. A successful configuration write SHALL distinguish persistence from runtime
activation and report `restart_required: true`. SemSource SHALL NOT claim that a newly desired source
is ingesting or that a removed desired source has stopped in the sealed running composition.
Source-file watches owned by already-running components SHALL continue to process their supported
content edits independently of component composition changes.

#### Scenario: A source is added after boot

- **WHEN** a source-management request persists a new source component configuration
- **THEN** the result reports that desired configuration changed and restart is required
- **AND** success does not assert that the source is running or indexed

#### Scenario: A desired source is removed

- **WHEN** a source-management request removes a component from next-boot desired configuration
- **THEN** the result reports restart required
- **AND** the running component's observed status is not silently reclassified as stopped

#### Scenario: A watched file changes inside an existing source

- **GIVEN** a source component was admitted at boot and supports file watching
- **WHEN** an owned source file changes
- **THEN** the existing watcher processes that edit without requiring composition replacement

### Requirement: Owned entities use the effective deployment authority

SemSource SHALL accept `platform_id` as the declared deployment stem, defaulting to `semsource` when
omitted. It SHALL validate the declared authority through the substrate contract and use the
framework-established effective authority after configuration startup. Different environments sharing
a broker SHALL use distinct stems; an environment label alone is not an isolation boundary.

SemSource SHALL construct owned entity identities as `org.platform.system.domain.type.instance`, using
the effective platform authority established by the substrate configuration manager. Constructors
SHALL receive that authority explicitly. Product names SHALL remain provenance, never substitute for
platform authority. A source declaring another org SHALL fail before ingestion unless a separately
admitted import contract authorizes it. Taxonomy scope SHALL remain correct before result limiting.

#### Scenario: First boot mints an authority

- **WHEN** the framework establishes an effective authority from the declared platform stem
- **THEN** every owned entity and owned relationship subject uses that effective authority
- **AND** constructors do not predict the minted suffix or rewrite a previously created ID

#### Scenario: Restart uses retained storage

- **WHEN** the same deployment restarts against its retained configuration store
- **THEN** it adopts the recorded authority and recreates the same intended owned entity IDs

#### Scenario: A taxonomy query crosses source systems

- **WHEN** a code or document lens searches its taxonomy across configured source systems
- **THEN** its scope selects the correct fourth-segment taxonomy before applying result limits
- **AND** it does not broaden to all deployment entities and filter only after truncation
