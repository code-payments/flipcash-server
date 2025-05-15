package memory

import (
	"testing"

	"github.com/code-payments/flipcash-server/push/tests"
)

func TestPush_MemoryPusher(t *testing.T) {
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*memory).reset()
	}
	tests.RunPusherTests(t, testStore, teardown)
}
