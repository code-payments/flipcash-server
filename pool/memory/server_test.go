package memory

import (
	"testing"

	account "github.com/code-payments/flipcash-server/account/memory"
	"github.com/code-payments/flipcash-server/pool/tests"
	profile "github.com/code-payments/flipcash-server/profile/memory"
)

func TestPool_MemoryServer(t *testing.T) {
	accounts := account.NewInMemory()
	profiles := profile.NewInMemory()
	testStore := NewInMemory()
	teardown := func() {
		testStore.(*InMemoryStore).reset()
	}
	tests.RunServerTests(t, accounts, testStore, profiles, teardown)
}
