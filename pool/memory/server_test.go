package memory

import (
	"testing"

	account "github.com/code-payments/flipcash-server/account/memory"
	"github.com/code-payments/flipcash-server/pool/tests"
)

func TestPool_MemoryServer(t *testing.T) {
	accounts := account.NewInMemory()
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*InMemoryStore).reset()
	}
	tests.RunServerTests(t, accounts, testStore, teardown)
}
