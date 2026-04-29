//go:build !e2e

package provider

// RuntimeRegistry returns production-configured providers. Scope 2 keeps the
// production registry empty unless real provider adapters register later.
func RuntimeRegistry() *Registry {
	return DefaultRegistry
}
