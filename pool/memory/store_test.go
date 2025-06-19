package memory

import (
	"testing"

	"github.com/code-payments/flipcash-server/pool/tests"
)

func TestPool_MemoryStore(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*InMemoryStore).reset()
	}
	tests.RunStoreTests(t, testStore, teardown)
}
