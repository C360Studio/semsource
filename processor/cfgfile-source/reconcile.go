package cfgfilesource

import (
	"crypto/sha256"
	"fmt"
	"sort"
	"strings"
	"sync"

	"github.com/c360studio/semengine/message"
)

// sentContent records, per entity ID, a fingerprint of the statements this
// process last handed to the publisher, so a republish is reconciled only
// when this process knows the entity's content changed (#336, D4). A first
// sighting — the seed, or a manifest created while watching — is not a known
// change: the stored entity may predate this process.
type sentContent struct {
	mu   sync.Mutex
	sums map[string][sha256.Size]byte
}

// differs reports whether this process sent id before with other statements.
func (s *sentContent) differs(id string, sum [sha256.Size]byte) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	prev, ok := s.sums[id]
	return ok && prev != sum
}

// record notes a publish the publisher accepted. Called only after Send
// succeeds, so a refused publish is compared again next time.
func (s *sentContent) record(id string, sum [sha256.Size]byte) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.sums == nil {
		s.sums = make(map[string][sha256.Size]byte)
	}
	s.sums[id] = sum
}

// fingerprint hashes a payload's statements order-independently, leaving out
// the timestamp, which differs on every parse.
func fingerprint(triples []message.Triple) [sha256.Size]byte {
	lines := make([]string, len(triples))
	for i, t := range triples {
		lines[i] = fmt.Sprintf("%s\x00%s\x00%v\x00%s\x00%s\x00%v",
			t.Subject, t.Predicate, t.Object, t.Datatype, t.Source, t.Confidence)
	}
	sort.Strings(lines)
	return sha256.Sum256([]byte(strings.Join(lines, "\n")))
}
