package memory

import (
	"testing"

	"github.com/code-payments/flipcash-server/push/tests"
)

func TestPush_MemoryStore(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*memory).reset()
	}
	tests.RunStoreTests(t, testStore, teardown)
}
