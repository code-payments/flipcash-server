package tests

import (
	"testing"

	"github.com/code-payments/flipcash-server/pool"
)

func RunStoreTests(t *testing.T, s pool.Store, teardown func()) {
	for _, tf := range []func(t *testing.T, s pool.Store){} {
		tf(t, s)
		teardown()
	}
}
