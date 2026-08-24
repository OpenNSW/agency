// Package scopes defines the Agency API's OAuth2 scope constants.
//
// These values must stay in sync with the Agency API resource server defined
// in nsw-srilanka's idp/resources/shared/resource-servers.json (agency:*
// namespace), seeded by that repo's idp/sample-resources.sh. Each constant
// corresponds to a permission derived from a resource + action handle in the
// IdP, following the pattern "agency:<resource>:<action>".
//
// Scope constants are defined here (not in internal/authz) so they can be
// imported from the composition root, tests, and any future service-layer
// checks without coupling the generic authz package to this application.
package scopes

const (
	// Application resource.
	ApplicationRead     = "agency:application:read"
	ApplicationReview   = "agency:application:review"
	ApplicationFeedback = "agency:application:feedback"
	ApplicationInject   = "agency:application:inject"

	// Consignment resource (read-only).
	ConsignmentRead = "agency:consignment:read"

	// Storage resource.
	StorageRead  = "agency:storage:read"
	StorageWrite = "agency:storage:write"

	// Profile resource (the caller's own user profile).
	ProfileRead = "agency:profile:read"
)
