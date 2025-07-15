package localization

import (
	"golang.org/x/text/language"
	"golang.org/x/text/message"
	"golang.org/x/text/number"

	codecurrency "github.com/code-payments/code-server/pkg/currency"
)

var symbolByCurrency = map[codecurrency.Code]string{
	codecurrency.AED: "د.إ",
	codecurrency.AFN: "؋",
	codecurrency.ALL: "Lek",
	codecurrency.ANG: "ƒ",
	codecurrency.AOA: "Kz",
	codecurrency.ARS: "$",
	codecurrency.AUD: "$",
	codecurrency.AWG: "ƒ",
	codecurrency.AZN: "₼",
	codecurrency.BAM: "KM",
	codecurrency.BDT: "৳",
	codecurrency.BBD: "$",
	codecurrency.BGN: "лв",
	codecurrency.BMD: "$",
	codecurrency.BND: "$",
	codecurrency.BOB: "$b",
	codecurrency.BRL: "R$",
	codecurrency.BSD: "$",
	codecurrency.BWP: "P",
	codecurrency.BYN: "Br",
	codecurrency.BZD: "BZ$",
	codecurrency.CAD: "$",
	codecurrency.CHF: "CHF",
	codecurrency.CLP: "$",
	codecurrency.CNY: "¥",
	codecurrency.COP: "$",
	codecurrency.CRC: "₡",
	codecurrency.CUC: "$",
	codecurrency.CUP: "₱",
	codecurrency.CZK: "Kč",
	codecurrency.DKK: "kr",
	codecurrency.DOP: "RD$",
	codecurrency.EGP: "£",
	codecurrency.ERN: "£",
	codecurrency.EUR: "€",
	codecurrency.FJD: "$",
	codecurrency.FKP: "£",
	codecurrency.GBP: "£",
	codecurrency.GEL: "₾",
	codecurrency.GGP: "£",
	codecurrency.GHS: "¢",
	codecurrency.GIP: "£",
	codecurrency.GNF: "FG",
	codecurrency.GTQ: "Q",
	codecurrency.GYD: "$",
	codecurrency.HKD: "$",
	codecurrency.HNL: "L",
	codecurrency.HRK: "kn",
	codecurrency.HUF: "Ft",
	codecurrency.IDR: "Rp",
	codecurrency.ILS: "₪",
	codecurrency.IMP: "£",
	codecurrency.INR: "₹",
	codecurrency.IRR: "﷼",
	codecurrency.ISK: "kr",
	codecurrency.JEP: "£",
	codecurrency.JMD: "J$",
	codecurrency.JPY: "¥",
	codecurrency.KGS: "лв",
	codecurrency.KHR: "៛",
	codecurrency.KMF: "CF",
	codecurrency.KPW: "₩",
	codecurrency.KRW: "₩",
	codecurrency.KYD: "$",
	codecurrency.KZT: "лв",
	codecurrency.LAK: "₭",
	codecurrency.LBP: "£",
	codecurrency.LKR: "₨",
	codecurrency.LRD: "$",
	codecurrency.LTL: "Lt",
	codecurrency.LVL: "Ls",
	codecurrency.MGA: "Ar",
	codecurrency.MKD: "ден",
	codecurrency.MMK: "K",
	codecurrency.MNT: "₮",
	codecurrency.MUR: "₨",
	codecurrency.MXN: "$",
	codecurrency.MYR: "RM",
	codecurrency.MZN: "MT",
	codecurrency.NAD: "$",
	codecurrency.NGN: "₦",
	codecurrency.NIO: "C$",
	codecurrency.NOK: "kr",
	codecurrency.NPR: "₨",
	codecurrency.NZD: "$",
	codecurrency.OMR: "﷼",
	codecurrency.PAB: "B/.",
	codecurrency.PEN: "S/.",
	codecurrency.PHP: "₱",
	codecurrency.PKR: "₨",
	codecurrency.PLN: "zł",
	codecurrency.PYG: "Gs",
	codecurrency.QAR: "﷼",
	codecurrency.RON: "lei",
	codecurrency.RSD: "Дин.",
	codecurrency.RUB: "₽",
	codecurrency.RWF: "RF",
	codecurrency.SAR: "﷼",
	codecurrency.SBD: "$",
	codecurrency.SCR: "₨",
	codecurrency.SEK: "kr",
	codecurrency.SGD: "$",
	codecurrency.SHP: "£",
	codecurrency.SOS: "S",
	codecurrency.SRD: "$",
	codecurrency.SSP: "£",
	codecurrency.STD: "Db",
	codecurrency.SVC: "$",
	codecurrency.SYP: "£",
	codecurrency.THB: "฿",
	codecurrency.TOP: "T$",
	codecurrency.TRY: "₺",
	codecurrency.TTD: "TT$",
	codecurrency.TWD: "NT$",
	codecurrency.UAH: "₴",
	codecurrency.USD: "$",
	codecurrency.UYU: "$U",
	codecurrency.UZS: "лв",
	codecurrency.VND: "₫",
	codecurrency.XCD: "$",
	codecurrency.YER: "﷼",
	codecurrency.ZAR: "R",
	codecurrency.ZMW: "ZK",
}

// FormatFiat formats a currency amount into a string in the provided locale
func FormatFiat(locale language.Tag, currency codecurrency.Code, amount float64) string {
	isRtlScript := isRtlScript(locale)

	decimals := 2

	printer := message.NewPrinter(locale)
	amountAsDecimal := number.Decimal(amount, number.Scale(decimals))
	formattedAmount := printer.Sprint(amountAsDecimal)

	symbol := symbolByCurrency[currency]

	if isRtlScript {
		return formattedAmount + symbol
	}
	return symbol + formattedAmount
}
