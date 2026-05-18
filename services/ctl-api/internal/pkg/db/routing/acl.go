package routing

import (
	"fmt"

	"gorm.io/gorm"
)

// viewModel matches the ViewModel interface implemented by models backed by
// SQL views (see internal/pkg/db/plugins/views). We declare it locally to
// avoid a routing → views package dependency; Go's structural typing means
// any model implementing both methods satisfies it.
type viewModel interface {
	UseView() bool
	ViewVersion() string
}

// TableACL controls replica routing per table.
//
//   - Deny: tables that must always read from primary. Wins over
//     everything, including ForceACL and opt-ins. Use for write-recency
//     requirements (e.g. auth flows that read-after-write).
//
//   - Allow: tables validated for replica reads. Used by the routing
//     Plugin's ForceACL mode to auto-route these tables to the replica
//     without requiring caller opt-in. With ForceACL off, Allow has no
//     effect — it's a registry of "we trust these tables", waiting to be
//     activated.
//
// Allow is NOT a restrictive whitelist: tables not in Allow are still
// eligible for replica reads through the normal opt-in paths (HTTP
// middleware, scope helpers, @replica-read activities, WithReplica).
// Only Deny restricts.
//
// The zero value (nil ACL or empty maps) is "no constraints" — every
// table is replica-eligible via opt-in, nothing is force-promoted.
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

// AllowsReplica reports whether queries against the given table may be
// routed to the replica. Only Deny restricts — Allow is a "validated"
// list used by ForceACL and does not narrow the eligible set.
func (a *TableACL) AllowsReplica(table string) bool {
	if a == nil || table == "" {
		return true
	}
	_, denied := a.Deny[table]
	return !denied
}

// IsExplicitlyAllowed reports whether the table is a named entry in the
// Allow list (not the looser "Allow empty = everything allowed" rule).
// Used by the force-ACL mode to auto-route a query to the replica when the
// caller hasn't otherwise opted in.
func (a *TableACL) IsExplicitlyAllowed(table string) bool {
	if a == nil || table == "" {
		return false
	}
	_, ok := a.Allow[table]
	return ok
}

// ACLBuilder constructs a TableACL from GORM model values, resolving each
// model's underlying table name through the open *gorm.DB. This is the
// preferred way to register models so we don't have to hand-maintain a list
// of string table names — renames flow through automatically.
//
//	acl := routing.NewACLBuilder(db).
//	    Deny(&app.Org{}, &app.Install{}).
//	    Allow(&app.AppConfig{}).
//	    Build()
//
// The builder uses gorm.Statement.Parse, which respects custom
// TableName() methods as well as the DB's NamingStrategy — the same
// resolution GORM uses at query time, so the ACL matches reality.
type ACLBuilder struct {
	db    *gorm.DB
	allow map[string]struct{}
	deny  map[string]struct{}
}

func NewACLBuilder(db *gorm.DB) *ACLBuilder {
	return &ACLBuilder{
		db:    db,
		allow: make(map[string]struct{}),
		deny:  make(map[string]struct{}),
	}
}

// Allow registers models that are eligible for replica routing. When the
// allow-list is non-empty, every other table is implicitly denied.
//
// If a model implements the ViewModel interface (UseView, ViewVersion)
// from the views plugin, both the base table name AND the current view
// name are registered — that way queries hitting either form (the view
// path is the default; scopes.WithDisableViews falls back to the base
// table) are caught by the ACL.
func (b *ACLBuilder) Allow(models ...interface{}) *ACLBuilder {
	for _, m := range models {
		for _, t := range tableNamesFor(b.db, m) {
			b.allow[t] = struct{}{}
		}
	}
	return b
}

// Deny registers models that must always read from the primary, even when
// allow-listed. Deny wins over Allow. View-backed models register both the
// base table name and the current view name; see Allow for details.
func (b *ACLBuilder) Deny(models ...interface{}) *ACLBuilder {
	for _, m := range models {
		for _, t := range tableNamesFor(b.db, m) {
			b.deny[t] = struct{}{}
		}
	}
	return b
}

// Build returns the assembled TableACL. The builder may be reused after
// Build (the returned ACL is a snapshot).
func (b *ACLBuilder) Build() *TableACL {
	acl := &TableACL{
		Allow: make(map[string]struct{}, len(b.allow)),
		Deny:  make(map[string]struct{}, len(b.deny)),
	}
	for t := range b.allow {
		acl.Allow[t] = struct{}{}
	}
	for t := range b.deny {
		acl.Deny[t] = struct{}{}
	}
	return acl
}

// tableNamesFor returns every SQL identifier the model can show up as in
// generated queries. For plain models that's one entry (the table). For
// ViewModel-implementing models with UseView()==true it's two entries:
// the base table (used by writes and queries that opt out of the view via
// scopes.WithDisableViews) and the current view name (the default read
// path for that model).
func tableNamesFor(db *gorm.DB, model interface{}) []string {
	base := ResolveTable(db, model)
	if base == "" {
		return nil
	}
	out := []string{base}
	if vm, ok := model.(viewModel); ok && vm.UseView() {
		// Format mirrors internal/pkg/db/plugins/views/views.go — keep
		// this in sync with the views plugin's table rewrite logic.
		out = append(out, fmt.Sprintf("%s_view_%s", base, vm.ViewVersion()))
	}
	return out
}

// ResolveTable returns the SQL table name GORM would use for the given
// model. It uses gorm.Statement.Parse (the same path GORM takes when
// building queries) so custom TableName() methods and the DB's
// NamingStrategy are honored. Returns "" if the model can't be parsed.
func ResolveTable(db *gorm.DB, model interface{}) string {
	stmt := &gorm.Statement{DB: db}
	if err := stmt.Parse(model); err != nil {
		// Fall back to a direct TableName() call so callers passing a
		// model with a broken/unparseable schema (rare) still get a
		// useful name.
		if t, ok := model.(interface{ TableName() string }); ok {
			return t.TableName()
		}
		return ""
	}
	if stmt.Schema == nil {
		return ""
	}
	return stmt.Schema.Table
}
