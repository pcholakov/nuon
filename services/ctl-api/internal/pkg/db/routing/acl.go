package routing

// TableACL controls which tables are eligible for replica routing.
//
// The zero value is the "everything allowed" policy: no tables are denied and
// no allow-list is configured, so every read is replica-eligible (subject to
// context opt-in / opt-out). This matches the rollout plan of sending all
// queries to the replica by default.
//
// Deny takes precedence over Allow. When Allow is non-empty, only listed
// tables are replica-eligible; everything else is forced to primary.
type TableACL struct {
	Allow map[string]struct{}
	Deny  map[string]struct{}
}

// NewTableACL builds an ACL from string slices for ergonomic construction.
func NewTableACL(allow, deny []string) *TableACL {
	acl := &TableACL{
		Allow: make(map[string]struct{}, len(allow)),
		Deny:  make(map[string]struct{}, len(deny)),
	}
	for _, t := range allow {
		acl.Allow[t] = struct{}{}
	}
	for _, t := range deny {
		acl.Deny[t] = struct{}{}
	}
	return acl
}

// AllowsReplica reports whether queries against the given table may be routed
// to the replica. A nil ACL or empty table name allows replica routing.
func (a *TableACL) AllowsReplica(table string) bool {
	if a == nil || table == "" {
		return true
	}
	if _, denied := a.Deny[table]; denied {
		return false
	}
	if len(a.Allow) > 0 {
		_, ok := a.Allow[table]
		return ok
	}
	return true
}
