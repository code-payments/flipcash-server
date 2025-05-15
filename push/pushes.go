package push

import (
	"context"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
)

var usdcAmountPrinter = message.NewPrinter(language.English)

func SendDepositReceivedPush(ctx context.Context, pusher Pusher, user *commonpb.UserId, quarks uint64) error {
	title := "Deposit Received"
	body := usdcAmountPrinter.Sprintf("%.2f USDC was deposited into your account", float64(quarks)/float64(codecommon.CoreMintQuarksPerUnit))
	return pusher.SendBasicPushes(ctx, title, body, user)
}
