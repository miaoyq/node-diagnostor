package validator

import "context"

// Scope defines the data collection scope
type Scope string

const (
	ScopeLocalOnly         Scope = "local_only"
	ScopeClusterSupplement Scope = "cluster_supplement"
	ScopeAuto              Scope = "auto"
)

// ValidationResult represents the result of a validation check
type ValidationResult struct {
	Valid   bool
	Skipped bool
	Reason  string
	Details map[string]interface{}
}

// Validator defines the interface for data scope validation
type Validator interface {
	// ValidateCollector validates if a collector should run based on scope
	ValidateCollector(ctx context.Context, collectorName string, scope Scope) (*ValidationResult, error)

	// ValidateDataType validates if a data type should be collected
	ValidateDataType(ctx context.Context, dataType string, scope Scope) (*ValidationResult, error)

	// GetLocalOnlyTypes returns all local-only data types
	GetLocalOnlyTypes() []string

	// GetClusterTypes returns all cluster supplement data types
	GetClusterTypes() []string

	// IsLocalOnly checks if a data type is local-only
	IsLocalOnly(dataType string) bool

	// IsClusterSupplement checks if a data type is cluster supplement
	IsClusterSupplement(dataType string) bool
}

// Cache defines the interface for validation result caching
type Cache interface {
	// Get retrieves a cached validation result
	Get(ctx context.Context, key string) (*ValidationResult, bool)

	// Set stores a validation result in cache
	Set(ctx context.Context, key string, result *ValidationResult) error

	// Invalidate removes a cached validation result
	Invalidate(ctx context.Context, key string) error

	// Clear clears all cached results
	Clear(ctx context.Context) error

	// Size returns the current cache size
	Size() int
}
