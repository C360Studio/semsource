# code-reference-resolution delta — multi-module source roots

## ADDED Requirements

### Requirement: Java type resolution spans a repo's module source roots

A Java repository built by Gradle or Maven has one source root **per module**
(`<module>/src/main/java`), so a fully-qualified name imported from a sibling
module does not lie under the referrer's own source root. Resolution SHALL
therefore probe the repo's other source roots of the same layout, after the
referrer's own root, which continues to take precedence. A name found under
exactly one root SHALL resolve; a name found under several SHALL resolve to
none of them.

An import that names an in-tree type but cannot be bound to exactly one file
SHALL NOT be reported as an external dependency, since it is neither resolved nor
third-party.

Discovery SHALL be by bounded probing of the repo layout rather than a full tree
walk, and SHALL depend only on repo content — never on filesystem enumeration
order.

#### Scenario: Type imported from a sibling module resolves

- **GIVEN** `lib/core/src/main/java/a/Repo.java` declares `public class Repo`
- **AND** `app/src/main/java/x/Svc.java` imports `a.Repo` and references it
- **WHEN** `Svc.java` is parsed
- **THEN** the reference resolves to `Repo`'s entity ID built against
  `lib/core/src/main/java/a/Repo.java`

#### Scenario: Test sources reach main sources

- **GIVEN** `app/src/main/java/a/Repo.java` declares `Repo`
- **AND** `app/src/test/java/a/RepoTest.java` references `Repo`
- **THEN** the reference resolves, because a referrer under one standard layout
  still reaches roots of the sibling layout

#### Scenario: The same fully-qualified name in two modules resolves to neither

- **GIVEN** `m1/src/main/java/a/Dup.java` and `m2/src/main/java/a/Dup.java` both
  declare `a.Dup`
- **WHEN** a third module imports `a.Dup`
- **THEN** no edge is emitted for that reference, and it is NOT reported as an
  `external:` third-party type

#### Scenario: A flat repo layout is unaffected

- **GIVEN** a repository whose sources are not under a `src/<phase>/java` layout
- **THEN** resolution behaves exactly as before, probing the referrer's own root
  and the repository root only
