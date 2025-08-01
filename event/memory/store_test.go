package memory

import (
	"testing"

	"github.com/code-payments/flipcash-server/event/tests"
)

func TestEvent_MemoryStore(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*InMemoryStore).reset()
	}
	tests.RunStoreTests(t, testStore, teardown)
}
