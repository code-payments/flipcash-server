package event

import (
	"sync"
	"time"

	"github.com/google/uuid"
	"go.uber.org/zap"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	eventpb "github.com/code-payments/flipcash-protobuf-api/generated/go/event/v1"

	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/model"
	"github.com/code-payments/flipcash-server/protoutil"
)

const (
	streamBufferSize = 64
	streamPingDelay  = 5 * time.Second
	streamTimeout    = time.Second

	maxEventBatchSize = 1024
)

// todo: requires distributed implementation
type Server struct {
	log *zap.Logger

	authz auth.Authorizer

	eventBus *Bus[*commonpb.UserId, *eventpb.Event]

	streamsMu sync.RWMutex
	streams   map[string]Stream[[]*eventpb.Event]

	eventpb.UnimplementedEventStreamingServer
}

func NewServer(log *zap.Logger, authz auth.Authorizer, eventBus *Bus[*commonpb.UserId, *eventpb.Event]) *Server {
	s := &Server{
		log: log,

		authz: authz,

		eventBus: eventBus,
	}

	eventBus.AddHandler(HandlerFunc[*commonpb.UserId, *eventpb.Event](s.OnEvent))

	return s
}

// todo: must be upgraded for a distributed environment
func (s *Server) StreamEvents(stream grpc.BidiStreamingServer[eventpb.StreamEventsRequest, eventpb.StreamEventsResponse]) error {
	ctx := stream.Context()

	req, err := protoutil.BoundedReceive[eventpb.StreamEventsRequest](
		ctx,
		stream,
		250*time.Millisecond,
	)
	if err != nil {
		return err
	}

	params := req.GetParams()
	if req.GetParams() == nil {
		return status.Error(codes.InvalidArgument, "missing parameters")
	}

	userID, err := s.authz.Authorize(ctx, params, &params.Auth)
	if err != nil {
		return err
	}

	streamID := uuid.New()

	log := s.log.With(
		zap.String("user_id", model.UserIDString(userID)),
		zap.String("stream_id", streamID.String()),
	)

	streamKey := model.UserIDString(userID)

	s.streamsMu.Lock()
	if existing, exists := s.streams[streamKey]; exists {
		delete(s.streams, streamKey)
		existing.Close()

		log.Info("Closed previous stream")
	}

	log.Debug("Initializing stream")

	ss := NewProtoEventStream(
		streamKey,
		streamBufferSize,
		func(events []*eventpb.Event) (*eventpb.EventBatch, bool) {
			if len(events) > maxEventBatchSize {
				log.Warn("Event batch size exceeds proto limit")
				return nil, false
			}

			if len(events) == 0 {
				return nil, false
			}

			cloned := protoutil.SliceClone(events)
			return &eventpb.EventBatch{Events: cloned}, true
		},
	)

	s.streams[streamKey] = ss
	s.streamsMu.Unlock()

	defer func() {
		s.streamsMu.Lock()

		log.Debug("Closing streamer")

		// We check to see if the current active stream is the one that we created.
		// If it is, we can just remove it since it's closed. Otherwise, we leave it
		// be, as another StreamEvents() call is handling it.
		liveStream := s.streams[streamKey]
		if liveStream == ss {
			delete(s.streams, streamKey)
		}

		s.streamsMu.Unlock()
	}()

	sendPingCh := time.After(0)
	streamHealthCh := protoutil.MonitorStreamHealth(ctx, log, stream, func(t *eventpb.StreamEventsRequest) bool {
		return t.GetPong() != nil
	})

	for {
		select {
		case batch, ok := <-ss.Channel():
			if !ok {
				log.Debug("Stream closed; ending stream")
				return status.Error(codes.Aborted, "stream closed")
			}

			log.Debug("Forwarding events")
			err = stream.Send(&eventpb.StreamEventsResponse{
				Type: &eventpb.StreamEventsResponse_Events{
					Events: batch,
				},
			})
			if err != nil {
				log.Info("Failed to forward chat update", zap.Error(err))
				return err
			}
		case <-sendPingCh:
			log.Debug("Sending ping to client")

			sendPingCh = time.After(streamPingDelay)

			err := stream.Send(&eventpb.StreamEventsResponse{
				Type: &eventpb.StreamEventsResponse_Ping{
					Ping: &commonpb.ServerPing{
						Timestamp: timestamppb.Now(),
						PingDelay: durationpb.New(streamPingDelay),
					},
				},
			})
			if err != nil {
				log.Debug("Stream is unhealthy; aborting")
				return status.Error(codes.Aborted, "terminating unhealthy stream")
			}
		case <-streamHealthCh:
			log.Debug("Stream is unhealthy; aborting")
			return status.Error(codes.Aborted, "terminating unhealthy stream")
		case <-ctx.Done():
			log.Debug("Stream context cancelled; ending stream")
			return status.Error(codes.Canceled, "")
		}
	}
}

func (s *Server) OnEvent(userID *commonpb.UserId, e *eventpb.Event) {
	streamKey := model.UserIDString(userID)
	s.streamsMu.RLock()
	stream, exists := s.streams[streamKey]
	s.streamsMu.RUnlock()

	if exists {
		cloned := proto.Clone(e).(*eventpb.Event)
		if err := stream.Notify([]*eventpb.Event{cloned}, streamTimeout); err != nil {
			s.log.Warn("Failed to send event", zap.Error(err))
		}
	}
}
