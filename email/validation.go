package email

import "regexp"

var (
	// A verification code must be a 4-10 digit string
	verificationCodePattern = regexp.MustCompile("^[0-9]{4,10}$")
)

// IsVerificationCode returns whether a string is a 4-10 digit numberical
// verification code.
func IsVerificationCode(code string) bool {
	return verificationCodePattern.Match([]byte(code))
}
