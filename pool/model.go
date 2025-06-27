package pool

import (
	"context"
	"encoding/base64"
	"time"

	"github.com/mr-tron/base58"
	"go.uber.org/zap"
	"google.golang.org/protobuf/proto"
	"google.golang.org/protobuf/types/known/timestamppb"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codedata "github.com/code-payments/code-server/pkg/code/data"
	codeintent "github.com/code-payments/code-server/pkg/code/data/intent"
	"github.com/code-payments/flipcash-server/auth"
	"github.com/code-payments/flipcash-server/model"
)

var (
	verifiedMetadataAuthn = auth.NewKeyPairAuthenticator()
)

type Resolution int

const (
	ResolutionUnknown Resolution = iota
	ResolutionRefunded
	ResolutionYes
	ResolutionNo
)

type Pool struct {
	ID                 *poolpb.PoolId
	CreatorID          *commonpb.UserId
	Name               string
	BuyInCurrency      string
	BuyInAmount        float64
	FundingDestination *commonpb.PublicKey
	IsOpen             bool
	Resolution         Resolution
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
		Resolution:         ToResolution(proto.Resolution),
		CreatedAt:          proto.CreatedAt.AsTime(),
		Signature:          signature,
	}

	if proto.ClosedAt != nil {
		closedAt := proto.ClosedAt.AsTime()
		model.ClosedAt = &closedAt
	}

	return model
}

func (p *Pool) HasResolution() bool {
	return p.Resolution != ResolutionUnknown
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
		Resolution:         p.Resolution,
		CreatedAt:          p.CreatedAt,
		Signature:          proto.Clone(p.Signature).(*commonpb.Signature),
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
			Resolution:         p.Resolution.ToProto(),
			CreatedAt:          timestamppb.New(p.CreatedAt),
		},
		RendezvousSignature: p.Signature,
	}
	if p.ClosedAt != nil {
		proto.VerifiedMetadata.ClosedAt = timestamppb.New(*p.ClosedAt)
	}
	return proto
}

func ToResolution(proto *poolpb.Resolution) Resolution {
	if proto == nil {
		return ResolutionUnknown
	}

	switch typed := proto.Kind.(type) {
	case *poolpb.Resolution_RefundResolution:
		return ResolutionRefunded
	case *poolpb.Resolution_BooleanResolution:
		if typed.BooleanResolution {
			return ResolutionYes
		}
		return ResolutionNo

	}

	return ResolutionUnknown
}

func (r *Resolution) ToProto() *poolpb.Resolution {
	if r == nil {
		return nil
	}

	switch *r {
	case ResolutionRefunded:
		return &poolpb.Resolution{
			Kind: &poolpb.Resolution_RefundResolution{
				RefundResolution: &poolpb.Resolution_Refund{},
			},
		}
	case ResolutionNo:
		return &poolpb.Resolution{
			Kind: &poolpb.Resolution_BooleanResolution{
				BooleanResolution: false,
			},
		}
	case ResolutionYes:
		return &poolpb.Resolution{
			Kind: &poolpb.Resolution_BooleanResolution{
				BooleanResolution: true,
			},
		}
	}

	return nil
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
	IsIntentSubmitted bool
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
		IsIntentSubmitted: false,
		Signature:         signature,
	}
}

func (b *Bet) IsPaid(ctx context.Context, codeData codedata.Provider, pools Store, pool *Pool) (bool, error) {
	if b.IsIntentSubmitted {
		return true, nil
	}

	intentId, err := codecommon.NewAccountFromPublicKeyBytes(b.ID.Value)
	if err != nil {
		return false, err
	}

	fundingDestinationAccout, err := codecommon.NewAccountFromPublicKeyBytes(pool.FundingDestination.Value)
	if err != nil {
		return false, err
	}

	intentRecord, err := codeData.GetIntent(ctx, intentId.PublicKey().ToBase58())
	switch err {
	case nil:
	case codeintent.ErrIntentNotFound:
		return false, nil
	default:
		return false, err
	}

	if intentRecord.IntentType != codeintent.SendPublicPayment {
		return false, nil
	}
	if intentRecord.SendPublicPaymentMetadata.DestinationTokenAccount != fundingDestinationAccout.PublicKey().ToBase58() {
		return false, nil
	}
	if string(intentRecord.SendPublicPaymentMetadata.ExchangeCurrency) != pool.BuyInCurrency {
		return false, nil
	}
	if intentRecord.SendPublicPaymentMetadata.NativeAmount != pool.BuyInAmount {
		return false, nil
	}
	if intentRecord.State == codeintent.StateRevoked {
		return false, nil
	}

	err = pools.MarkBetAsPaid(ctx, b.ID)
	if err != nil {
		return false, err
	}

	return true, nil
}

func (b *Bet) Clone() *Bet {
	return &Bet{
		PoolID:            proto.Clone(b.PoolID).(*poolpb.PoolId),
		ID:                proto.Clone(b.ID).(*poolpb.BetId),
		UserID:            proto.Clone(b.UserID).(*commonpb.UserId),
		SelectedOutcome:   b.SelectedOutcome,
		PayoutDestination: proto.Clone(b.PayoutDestination).(*commonpb.PublicKey),
		Ts:                b.Ts,
		IsIntentSubmitted: b.IsIntentSubmitted,
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

func VerifyPoolSignature(log *zap.Logger, signedPool *poolpb.SignedPoolMetadata, signature *commonpb.Signature) bool {
	isVerified := verifySignedMetadata(signedPool.Id, signedPool, signature)
	if !isVerified {
		marshalled, err := proto.Marshal(signedPool)
		if err != nil {
			return false
		}
		log.With(zap.String("base64_value", base64.StdEncoding.EncodeToString(marshalled))).Info("pool signature verification failed")
	}
	return isVerified
}

func VerifyBetSignature(log *zap.Logger, poolID *poolpb.PoolId, signedBet *poolpb.SignedBetMetadata, signature *commonpb.Signature) bool {
	isVerified := verifySignedMetadata(poolID, signedBet, signature)
	if !isVerified {
		marshalled, err := proto.Marshal(signedBet)
		if err != nil {
			return false
		}
		log.With(zap.String("base64_value", base64.StdEncoding.EncodeToString(marshalled))).Info("bet signature verification failed")
	}
	return isVerified
}

func verifySignedMetadata(poolID *poolpb.PoolId, msg proto.Message, signature *commonpb.Signature) bool {
	err := verifiedMetadataAuthn.Verify(context.Background(), msg, &commonpb.Auth{
		Kind: &commonpb.Auth_KeyPair_{
			KeyPair: &commonpb.Auth_KeyPair{
				PubKey:    &commonpb.PublicKey{Value: poolID.Value},
				Signature: signature,
			},
		},
	})
	return err == nil
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

func (r Resolution) String() string {
	switch r {
	case ResolutionRefunded:
		return "refunded"
	case ResolutionYes:
		return "yes"
	case ResolutionNo:
		return "no"
	}
	return "unknown"
}
