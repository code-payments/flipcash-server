package pool

import (
	"context"
	"sync"
	"time"

	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	eventpb "github.com/code-payments/flipcash-protobuf-api/generated/go/event/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/flipcash-server/event"
	"github.com/code-payments/flipcash-server/model"
	"github.com/code-payments/flipcash-server/push"
)

// todo: add tests
type StaleEventDetector struct {
	mu                   sync.Mutex
	latestBetStateByPool map[string]*poolpb.BetSummary
}

func NewStaleEventDetector() event.StaleEventDetector[*eventpb.Event] {
	return &StaleEventDetector{
		latestBetStateByPool: make(map[string]*poolpb.BetSummary),
	}
}

func (d *StaleEventDetector) ShouldDrop(e *eventpb.Event) bool {
	switch typed := e.Type.(type) {
	case *eventpb.Event_PoolBetUpdate:
		d.mu.Lock()
		defer d.mu.Unlock()

		key := PoolIDString(typed.PoolBetUpdate.PoolId)

		previouslyObserved, ok := d.latestBetStateByPool[key]
		if !ok {
			d.latestBetStateByPool[key] = proto.Clone(typed.PoolBetUpdate.BetSummary).(*poolpb.BetSummary)
			return false
		}

		previouslyObservedVotes := previouslyObserved.GetBooleanSummary().NumYes + previouslyObserved.GetBooleanSummary().NumNo
		votesInEvent := typed.PoolBetUpdate.BetSummary.GetBooleanSummary().NumYes + typed.PoolBetUpdate.BetSummary.GetBooleanSummary().NumNo
		if previouslyObservedVotes >= votesInEvent {
			return true
		}

		d.latestBetStateByPool[key] = proto.Clone(typed.PoolBetUpdate.BetSummary).(*poolpb.BetSummary)
		return false
	}

	return false
}

func (h *IntentHandler) OnSuccessfulBetPayment(ctx context.Context, intentRecord *codeintent.Record) error {
	intentID, err := codecommon.NewAccountFromPublicKeyString(intentRecord.IntentId)
	if err != nil {
		return err
	}

	bet, err := h.pools.GetBetByID(ctx, &poolpb.BetId{Value: intentID.PublicKey().ToBytes()})
	if err != nil {
		return err
	}

	bettingPool, err := h.pools.GetPoolByID(ctx, bet.PoolID)
	if err != nil {
		return err
	}

	ts := time.Now()
	betSummary, bets, err := GetBetSummary(ctx, h.pools, h.codeData, bettingPool)
	if err != nil {
		return err
	}

	usersToNotify := make(map[string]*commonpb.UserId)
	usersToNotify[model.UserIDString(bettingPool.CreatorID)] = bettingPool.CreatorID
	for _, bet := range bets {
		usersToNotify[model.UserIDString(bet.UserID)] = bet.UserID
	}

	userEvents := make([]*eventpb.UserEvent, 0)
	for _, userID := range usersToNotify {
		userEvents = append(userEvents, &eventpb.UserEvent{
			UserId: userID,
			Event: &eventpb.Event{
				Id: event.MustGenerateEventID(),
				Ts: timestamppb.New(ts),
				Type: &eventpb.Event_PoolBetUpdate{
					PoolBetUpdate: &eventpb.PoolBetUpdateEvent{
						PoolId:     bettingPool.ID,
						BetSummary: betSummary,
					},
				},
			},
		})
	}
	return h.eventForwarder.ForwardUserEvents(ctx, userEvents...)
}

func (s *Server) notifyPoolResolution(ctx context.Context, poolID *poolpb.PoolId, ts time.Time) error {
	pool, err := s.pools.GetPoolByID(ctx, poolID)
	if err != nil {
		return err
	}

	verifiedProtoPool := pool.ToProto().VerifiedMetadata

	betSummary, bets, err := GetBetSummary(ctx, s.pools, s.codeData, pool)
	if err != nil {
		return err
	}

	var winners []*commonpb.UserId
	var losers []*commonpb.UserId
	var refundedUsers []*commonpb.UserId
	var winOutcome *poolpb.UserPoolSummary_WinOutcome
	var loseOutcome *poolpb.UserPoolSummary_LoseOutcome
	var refundOutcome *poolpb.UserPoolSummary_RefundOutcome
	for _, bet := range bets {
		userSummary, err := getUserSummaryWithCachedBetMetadata(bet.UserID, pool, betSummary, bets)
		if err != nil {
			return err
		}
		switch typed := userSummary.Outcome.(type) {
		case *poolpb.UserPoolSummary_Win:
			winners = append(winners, bet.UserID)
			winOutcome = typed.Win
		case *poolpb.UserPoolSummary_Lose:
			losers = append(losers, bet.UserID)
			loseOutcome = typed.Lose
		case *poolpb.UserPoolSummary_Refund:
			refundedUsers = append(refundedUsers, bet.UserID)
			refundOutcome = typed.Refund
		}
	}

	if len(winners) > 0 {
		for _, winner := range winners {
			s.eventBus.OnEvent(winner, &eventpb.Event{
				Id: event.MustGenerateEventID(),
				Ts: timestamppb.New(ts),
				Type: &eventpb.Event_PoolResolved{
					PoolResolved: &eventpb.PoolResolvedEvent{
						Pool:       verifiedProtoPool,
						BetSummary: betSummary,
						UserSummary: &poolpb.UserPoolSummary{
							Outcome: &poolpb.UserPoolSummary_Win{
								Win: winOutcome,
							},
						},
					},
				},
			})
		}

		go push.SendWinBettingPoolPushes(ctx, s.pusher, pool.Name, winOutcome.AmountWon, winners...)
	}

	if len(losers) > 0 {
		for _, loser := range losers {
			s.eventBus.OnEvent(loser, &eventpb.Event{
				Id: event.MustGenerateEventID(),
				Ts: timestamppb.New(ts),
				Type: &eventpb.Event_PoolResolved{
					PoolResolved: &eventpb.PoolResolvedEvent{
						Pool:       verifiedProtoPool,
						BetSummary: betSummary,
						UserSummary: &poolpb.UserPoolSummary{
							Outcome: &poolpb.UserPoolSummary_Lose{
								Lose: loseOutcome,
							},
						},
					},
				},
			})
		}

		go push.SendLostBettingPoolPushes(ctx, s.pusher, pool.Name, loseOutcome.AmountLost, losers...)
	}

	if len(refundedUsers) > 0 {
		for _, user := range refundedUsers {
			s.eventBus.OnEvent(user, &eventpb.Event{
				Id: event.MustGenerateEventID(),
				Ts: timestamppb.New(ts),
				Type: &eventpb.Event_PoolResolved{
					PoolResolved: &eventpb.PoolResolvedEvent{
						Pool:       verifiedProtoPool,
						BetSummary: betSummary,
						UserSummary: &poolpb.UserPoolSummary{
							Outcome: &poolpb.UserPoolSummary_Refund{
								Refund: refundOutcome,
							},
						},
					},
				},
			})
		}

		go push.SendTieBettingPoolPushes(ctx, s.pusher, pool.Name, refundedUsers...)
	}

	return nil
}
