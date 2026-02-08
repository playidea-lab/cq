package bridge

import "testing"

// TestBridgeInterfaceCompliance verifies that PythonBridge satisfies
// the Bridge interface at compile time.
func TestBridgeInterfaceCompliance(t *testing.T) {
	var _ Bridge = (*PythonBridge)(nil)

	// PythonBridge only implements Bridge (not ExtendedBridge).
	// GRPCBridge implements both, but is behind the "grpc" build tag.
}
