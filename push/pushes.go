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

	betWinEmojis  = []string{"😎", "🤠", "🙌", "🤌", "🔥", "😝", "😏", "🥳", "🫨", "😤", "🙀", "👐", "🤘", "💪", "👀", "🕺", "💃", "🍻", "🥂", "🏋️", "🤸‍♀️", "🤾‍♂️", "🏆", "🥇", "🎯", "🎉", "📈", "🥷", "🧙‍♂️", "👑", "🎯", "🔈", "🏁"}
	betLostEmojis = []string{"😅", "🙃", "😭", "😳", "😱", "🫣", "🫥", "😬", "🙄", "🥴", "🤓", "💩", "☠️", "✌️", "🤦‍♂️", "🤦‍♀️", "🤷‍♀️", "🤷", "💆‍♂️", "🙈", "🙊", "🦨", "☔️", "🥃", "🥊", "🎭", "🚑", "🚬", "🪠", "🚽", "🃏", "🏴‍☠️", "📉"}
)

func SendDepositReceivedPush(ctx context.Context, pusher Pusher, user *commonpb.UserId, quarks uint64) error {
	title := "Deposit Received"
	body := usdcAmountPrinter.Sprintf("You deposited $%.2f of USDC", float64(quarks)/float64(codecommon.CoreMintQuarksPerUnit))
	return pusher.SendBasicPushes(ctx, title, body, user)
}

func SendWinBettingPoolPushes(ctx context.Context, pusher Pusher, poolName string, amountWon *commonpb.FiatPaymentAmount, winners ...*commonpb.UserId) error {
	if amountWon.NativeAmount < 0.01 {
		return nil
	}
	title := fmt.Sprintf("You won %s", betWinEmojis[rand.IntN(len(betWinEmojis))])
	localizedNativeAmount, err := localization.FormatFiat(defaultLocale, codecurrency.Code(amountWon.Currency), amountWon.NativeAmount)
	if err != nil {
		return err
	}
	body := fmt.Sprintf(`You won %s on '%s'`, localizedNativeAmount, poolName)
	return pusher.SendBasicPushes(ctx, title, body, winners...)
}

func SendLostBettingPoolPushes(ctx context.Context, pusher Pusher, poolName string, amountLost *commonpb.FiatPaymentAmount, losers ...*commonpb.UserId) error {
	title := fmt.Sprintf("You lost %s", betLostEmojis[rand.IntN(len(betLostEmojis))])
	localizedNativeAmount, err := localization.FormatFiat(defaultLocale, codecurrency.Code(amountLost.Currency), amountLost.NativeAmount)
	if err != nil {
		return err
	}
	body := fmt.Sprintf(`You lost %s on '%s'`, localizedNativeAmount, poolName)
	return pusher.SendBasicPushes(ctx, title, body, losers...)
}
