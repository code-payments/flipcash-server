package memory

import (
	"testing"

	"github.com/code-payments/flipcash-server/pool/tests"
)

func TestPool_MemoryServer(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*InMemoryStore).reset()
	}
	tests.RunServerTests(t, testStore, teardown)
}
