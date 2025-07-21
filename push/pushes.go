package push

import (
	"context"
	"fmt"
	"math/rand/v2"

	"golang.org/x/text/language"
	"golang.org/x/text/message"

	commonpb "github.com/code-payments/flipcash-protobuf-api/generated/go/common/v1"
	"github.com/code-payments/flipcash-server/localization"

	codecommon "github.com/code-payments/code-server/pkg/code/common"
	codecurrency "github.com/code-payments/code-server/pkg/currency"
)

var (
	defaultLocale     = language.English
	usdcAmountPrinter = message.NewPrinter(defaultLocale)

	betWinEmojis  = []string{"😎", "🤠", "🙌", "🤌", "🔥", "😝", "😏", "🥳", "🫨", "😤", "🙀", "👐", "🤘", "💪", "👀", "🕺", "💃", "🍻", "🥂", "🏋️", "🤸‍♀️", "🤾‍♂️", "🏆", "🥇", "🎯", "🎉", "📈", "🥷", "🧙‍♂️", "👑", "🎯", "🔈", "🏁", "🤯"}
	betLostEmojis = []string{"😅", "🙃", "😭", "😳", "😱", "🫣", "🫥", "😬", "🙄", "🥴", "🤓", "💩", "☠️", "✌️", "🤦‍♂️", "🤦‍♀️", "🤷‍♀️", "🤷", "💆‍♂️", "🙈", "🙊", "🦨", "☔️", "🥃", "🥊", "🎭", "🚑", "🚬", "🪠", "🚽", "🃏", "🏴‍☠️", "📉"}
	betTieEmojis  = []string{"🫠"}
)

func SendDepositReceivedPush(ctx context.Context, pusher Pusher, user *commonpb.UserId, quarks uint64) error {
	title := "Deposit Received"
	body := usdcAmountPrinter.Sprintf(
		"You deposited $%.2f of USDC",
		float64(quarks)/float64(codecommon.CoreMintQuarksPerUnit),
	)
	return pusher.SendBasicPushes(ctx, title, body, user)
}

func SendWinBettingPoolPushes(ctx context.Context, pusher Pusher, poolName string, amountWon *commonpb.FiatPaymentAmount, winners ...*commonpb.UserId) error {
	if amountWon.NativeAmount < 0.01 {
		return nil
	}
	title := fmt.Sprintf(
		"You won! %s",
		betWinEmojis[rand.IntN(len(betWinEmojis))],
	)
	body := fmt.Sprintf(
		`You won %s on '%s'`,
		localization.FormatFiat(defaultLocale, codecurrency.Code(amountWon.Currency), amountWon.NativeAmount),
		poolName,
	)
	return pusher.SendBasicPushes(ctx, title, body, winners...)
}

func SendLostBettingPoolPushes(ctx context.Context, pusher Pusher, poolName string, amountLost *commonpb.FiatPaymentAmount, losers ...*commonpb.UserId) error {
	title := fmt.Sprintf(
		"You lost! %s",
		betLostEmojis[rand.IntN(len(betLostEmojis))],
	)
	body := fmt.Sprintf(
		`You lost %s on '%s'`,
		localization.FormatFiat(defaultLocale, codecurrency.Code(amountLost.Currency), amountLost.NativeAmount),
		poolName,
	)
	return pusher.SendBasicPushes(ctx, title, body, losers...)
}

func SendTieBettingPoolPushes(ctx context.Context, pusher Pusher, poolName string, participants ...*commonpb.UserId) error {
	title := fmt.Sprintf(
		"It's a tie! %s",
		betTieEmojis[rand.IntN(len(betTieEmojis))],
	)
	body := fmt.Sprintf(
		`Your buy in was returned for '%s'`,
		poolName,
	)
	return pusher.SendBasicPushes(ctx, title, body, participants...)
}
