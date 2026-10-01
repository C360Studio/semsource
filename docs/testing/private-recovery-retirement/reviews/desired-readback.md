# Independent review addendum — root retained config reader tests

APPROVED. Reviewer: removal_projection, independent of author removal_publisher.
Scope is only the new `cmd/semsource/desired_config_integration_test.go` (SHA256
7cc2f8b0cc2131f83c7770a58bb2091bdfb0e4b6017924480701414bd3bf372f).
No blocking findings or requested changes.

The tests call the existing root `readDesiredConfig` against real NATS using ConfigManager-owned
complete envelopes. They verify exact name/type/disabled state/raw config and handle sanitization;
missing keys; deliberate malformed bytes in test-owned storage; separate org and stem namespaces
with the same handle; missing authority and invalid namespace; and reader calls after closing a
separate real connection. An independent live client compares retained key sets, bytes, revisions
and message count before/after reads. Missing config storage and legacy recovery storage remain
absent. The fixture manager is stopped before measuring read-only behavior. No new recovery
mechanism, production change, arbitrary sleep, or external fixture dependency was added.

Independent validation: `review-desired-readback.log` and `review-desired-readback.cover`.
Three real-NATS integration tests pass with Go race detection (2.721s, exit 0).
`readDesiredConfig` statement coverage is 100.0%; the isolated command package aggregate is 2.8%,
not a claim that this focused run covers the whole composition root. Tagged vet, pinned revive,
gofmt and git diff --check pass.

This test-only addition does not alter the production tree at 5334716 or invalidate its final
binary/process evidence. It qualifies the existing config authority read seam; it does not claim
graph mutation outcome resolution, source-removal projection or positive reactivation.
