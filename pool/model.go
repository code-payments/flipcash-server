package pool

import (
	"crypto/ed25519"
	"errors"

	"github.com/gogo/protobuf/proto"
	"github.com/mr-tron/base58"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	poolpb "github.com/code-payments/flipcash-protobuf-api/generated/go/pool/v1"
)

type Pool struct {
	ID         *poolpb.PoolId
	Creator    *commonpb.UserId
	IsOpen     bool
	Resolution *bool
}

func ToPoolModel(proto *poolpb.SignedPoolMetadata, signature *commonpb.Signature) (*Pool, error) {
	return nil, errors.New("not implemented")
}

func (p *Pool) ToProto() (*poolpb.PoolMetadata, error) {
	return nil, errors.New("not implemented")
}

type Bet struct {
	PoolID          *poolpb.PoolId
	ID              *poolpb.BetId
	SelectedOutcome bool
}

func ToBetModel(poolID *poolpb.PoolId, proto *poolpb.SignedBetMetadata, signature *commonpb.Signature) (*Bet, error) {
	return nil, errors.New("not implemented")
}

func (b *Bet) ToProto() (*poolpb.BetMetadata, error) {
	return nil, errors.New("not implemented")
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

func PoolIDString(id *poolpb.PoolId) string {
	if id == nil {
		return "<nil>"
	}
	return base58.Encode(id.Value)
}

func BetIDString(id *poolpb.BetId) string {
	if id == nil {
		return "<nil>"
	}
	return base58.Encode(id.Value)
}
