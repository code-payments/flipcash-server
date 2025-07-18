package event

import (
	"context"

	eventpb "github.com/code-payments/flipcash-protobuf-api/generated/go/event/v1"
)

type Forwarder interface {
	ForwardUserEvents(ctx context.Context, events ...*eventpb.UserEvent) error
}
