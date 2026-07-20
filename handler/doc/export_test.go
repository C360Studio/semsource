package doc

// PassageHardMax re-exports the passage size bound to the external test package,
// so the passage-size contract is pinned against the real constant instead of a
// copy of its value that would silently drift the day the bound is tuned.
const PassageHardMax = passageHardMax
