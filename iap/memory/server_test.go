package memory

import (
	"testing"

	account "github.com/code-payments/flipcash-server/account/memory"
	"github.com/code-payments/flipcash-server/iap/tests"
)

func TestIAP_MemoryServer(t *testing.T) {
	pub, priv, err := GenerateKeyPair()
	if err != nil {
		t.Fatalf("error generating key pair: %v", err)
	}

	verifier := NewMemoryVerifier(pub)
	validReceiptFunc := func(msg string) string {
		return GenerateValidReceipt(priv, msg)
	}

	accounts := account.NewInMemory()
	iaps := NewInMemory()

	teardown := func() {
		iaps.(*InMemoryStore).reset()
	}

	tests.RunServerTests(t, accounts, iaps, verifier, validReceiptFunc, teardown)
}
