package memory

import (
	"testing"

	"github.com/code-payments/flipcash-server/push/tests"
)

func TestPush_MemoryServer(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*memory).reset()
	}
	tests.RunServerTests(t, testStore, teardown)
}
