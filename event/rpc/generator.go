package rpc

import (
	"go.uber.org/zap"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	eventpb "github.com/code-payments/flipcash-protobuf-api/generated/go/event/v1"

	"github.com/code-payments/flipcash-server/event"
)

type Generator struct {
	log      *zap.Logger
	eventBus *event.Bus[*commonpb.UserId, *eventpb.Event]
}

func NewGenerator(log *zap.Logger, eventBus *event.Bus[*commonpb.UserId, *eventpb.Event]) event.Generator {
	return &Generator{
		log:      log,
		eventBus: eventBus,
	}
}
