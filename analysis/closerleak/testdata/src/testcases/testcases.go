// Package testcases contains test cases for the closerleak analyzer.
package testcases

import "io"

// mockProvider is a type with a Close method for testing.
type mockProvider struct{}

func (m *mockProvider) Close() error { return nil }

// NewMockProvider creates a mock provider.
func NewMockProvider() (*mockProvider, error) {
	return &mockProvider{}, nil
}

// LeakyFunction demonstrates a resource leak - no Close() called.
func LeakyFunction() error {
	provider, err := NewMockProvider() // want "potential resource leak: provider .* is never closed"
	if err != nil {
		return err
	}
	_ = provider
	return nil
}

// CorrectDeferFunction demonstrates correct usage with defer.
func CorrectDeferFunction() error {
	provider, err := NewMockProvider()
	if err != nil {
		return err
	}
	defer provider.Close()
	return nil
}

// CorrectExplicitClose demonstrates correct usage with explicit Close().
func CorrectExplicitClose() error {
	provider, err := NewMockProvider()
	if err != nil {
		return err
	}
	provider.Close()
	return nil
}

// ReturnedProvider is not a leak - caller is responsible.
func ReturnedProvider() (*mockProvider, error) {
	provider, err := NewMockProvider()
	if err != nil {
		return nil, err
	}
	return provider, nil
}

// BlankIdentifier should not report (assigned to _).
func BlankIdentifier() error {
	_, err := NewMockProvider()
	return err
}

// ReaderLeak demonstrates io.ReadCloser leak.
func ReaderLeak(r io.ReadCloser) {
	// This should not report - r is a parameter, not created here
	_ = r
}
