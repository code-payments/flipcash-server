package pool

import (
	"crypto/ed25519"
	"time"

	"github.com/mr-tron/base58"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"
	"github.com/code-payments/flipcash-server/model"
)

type Pool struct {
	ID                 *poolpb.PoolId
	CreatorID          *commonpb.UserId
	Name               string
	BuyInCurrency      string
	BuyInAmount        float64
	FundingDestination *commonpb.PublicKey
	IsOpen             bool
	Resolution         *bool
	CreatedAt          time.Time
	ClosedAt           *time.Time
	Signature          *commonpb.Signature
}

func ToPoolModel(proto *poolpb.SignedPoolMetadata, signature *commonpb.Signature) *Pool {
	model := &Pool{
		ID:                 proto.Id,
		CreatorID:          proto.Creator,
		Name:               proto.Name,
		BuyInCurrency:      proto.BuyIn.Currency,
		BuyInAmount:        proto.BuyIn.NativeAmount,
		FundingDestination: proto.FundingDestination,
		IsOpen:             proto.IsOpen,
		CreatedAt:          proto.CreatedAt.AsTime(),
		Signature:          signature,
	}

	if proto.Resolution != nil {
		resolution := proto.Resolution.GetBooleanResolution()
		model.Resolution = &resolution
	}

	if proto.ClosedAt != nil {
		closedAt := proto.ClosedAt.AsTime()
		model.ClosedAt = &closedAt
	}

	return model
}

func (p *Pool) Clone() *Pool {
	cloned := &Pool{
		ID:                 proto.Clone(p.ID).(*poolpb.PoolId),
		CreatorID:          proto.Clone(p.CreatorID).(*commonpb.UserId),
		Name:               p.Name,
		BuyInCurrency:      p.BuyInCurrency,
		BuyInAmount:        p.BuyInAmount,
		FundingDestination: proto.Clone(p.FundingDestination).(*commonpb.PublicKey),
		IsOpen:             p.IsOpen,
		CreatedAt:          p.CreatedAt,
		Signature:          proto.Clone(p.Signature).(*commonpb.Signature),
	}

	if p.Resolution != nil {
		value := *p.Resolution
		cloned.Resolution = &value
	}

	if p.ClosedAt != nil {
		value := *p.ClosedAt
		cloned.ClosedAt = &value
	}

	return cloned
}

func ClonePools(pools []*Pool) []*Pool {
	cloned := make([]*Pool, len(pools))
	for i, pool := range pools {
		cloned[i] = pool.Clone()
	}
	return cloned
}

func (p *Pool) ToProto() *poolpb.PoolMetadata {
	proto := &poolpb.PoolMetadata{
		VerifiedMetadata: &poolpb.SignedPoolMetadata{
			Id:      p.ID,
			Creator: p.CreatorID,
			Name:    p.Name,
			BuyIn: &commonpb.FiatPaymentAmount{
				Currency:     p.BuyInCurrency,
				NativeAmount: p.BuyInAmount,
			},
			FundingDestination: p.FundingDestination,
			IsOpen:             p.IsOpen,
			CreatedAt:          timestamppb.New(p.CreatedAt),
		},
		RendezvousSignature: p.Signature,
	}
	if p.Resolution != nil {
		proto.VerifiedMetadata.Resolution = &poolpb.Resolution{
			Kind: &poolpb.Resolution_BooleanResolution{
				BooleanResolution: *p.Resolution,
			},
		}
	}
	if p.ClosedAt != nil {
		proto.VerifiedMetadata.ClosedAt = timestamppb.New(*p.ClosedAt)
	}
	return proto
}

type Member struct {
	ID     []byte
	UserID *commonpb.UserId
	PoolID *poolpb.PoolId
}

func (m *Member) Clone() *Member {
	clonedID := make([]byte, len(m.ID))
	copy(clonedID, m.ID)
	return &Member{
		ID:     clonedID,
		UserID: proto.Clone(m.UserID).(*commonpb.UserId),
		PoolID: proto.Clone(m.PoolID).(*poolpb.PoolId),
	}
}

func CloneMembers(members []*Member) []*Member {
	cloned := make([]*Member, len(members))
	for i, member := range members {
		cloned[i] = member.Clone()
	}
	return cloned
}

type Bet struct {
	PoolID            *poolpb.PoolId
	ID                *poolpb.BetId
	UserID            *commonpb.UserId
	SelectedOutcome   bool
	PayoutDestination *commonpb.PublicKey
	Ts                time.Time
	Signature         *commonpb.Signature
}

func ToBetModel(poolID *poolpb.PoolId, proto *poolpb.SignedBetMetadata, signature *commonpb.Signature) *Bet {
	return &Bet{
		PoolID:            poolID,
		ID:                proto.BetId,
		UserID:            proto.UserId,
		SelectedOutcome:   proto.SelectedOutcome.GetBooleanOutcome(),
		PayoutDestination: proto.PayoutDestination,
		Ts:                proto.Ts.AsTime(),
		Signature:         signature,
	}
}

func (b *Bet) Clone() *Bet {
	return &Bet{
		PoolID:            proto.Clone(b.PoolID).(*poolpb.PoolId),
		ID:                proto.Clone(b.ID).(*poolpb.BetId),
		UserID:            proto.Clone(b.UserID).(*commonpb.UserId),
		SelectedOutcome:   b.SelectedOutcome,
		PayoutDestination: proto.Clone(b.PayoutDestination).(*commonpb.PublicKey),
		Ts:                b.Ts,
		Signature:         proto.Clone(b.Signature).(*commonpb.Signature),
	}
}

func CloneBets(bets []*Bet) []*Bet {
	cloned := make([]*Bet, len(bets))
	for i, bet := range bets {
		cloned[i] = bet.Clone()
	}
	return cloned
}

func (b *Bet) ToProto() *poolpb.BetMetadata {
	return &poolpb.BetMetadata{
		VerifiedMetadata: &poolpb.SignedBetMetadata{
			BetId:  b.ID,
			UserId: b.UserID,
			SelectedOutcome: &poolpb.BetOutcome{
				Kind: &poolpb.BetOutcome_BooleanOutcome{
					BooleanOutcome: b.SelectedOutcome,
				},
			},
			PayoutDestination: b.PayoutDestination,
			Ts:                timestamppb.New(b.Ts),
		},
		RendezvousSignature: b.Signature,
	}
}

func VerifyPoolSignature(signedPool *poolpb.SignedPoolMetadata, signature *commonpb.Signature) bool {
	marshalled, err := proto.Marshal(signedPool)
	if err != nil {
		return false
	}
	return ed25519.Verify(signedPool.Id.Value, marshalled, signature.Value)
}

func VerifyBetSignature(poolID *poolpb.PoolId, signedBet *poolpb.SignedBetMetadata, signature *commonpb.Signature) bool {
	marshalled, err := proto.Marshal(signedBet)
	if err != nil {
		return false
	}
	return ed25519.Verify(poolID.Value, marshalled, signature.Value)
}

func ToPoolID(keyPair model.KeyPair) *poolpb.PoolId {
	return &poolpb.PoolId{Value: keyPair.Proto().Value}
}

func PoolIDString(id *poolpb.PoolId) string {
	if id == nil {
		return "<nil>"
	}
	return base58.Encode(id.Value)
}

func ToBetID(keyPair model.KeyPair) *poolpb.BetId {
	return &poolpb.BetId{Value: keyPair.Proto().Value}
}

func BetIDString(id *poolpb.BetId) string {
	if id == nil {
		return "<nil>"
	}
	return base58.Encode(id.Value)
}
