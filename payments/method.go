package payments

// Method — способ, которым клиент оплачивает платёж.
type Method string

const (
	MethodApplePay  Method = "applepay"
	MethodGooglePay Method = "googlepay"
	MethodCard      Method = "card"
	MethodSBP       Method = "sbp"
)

// IsValid сообщает, поддерживается ли этот способ оплаты.
func (m Method) IsValid() bool {
	switch m {
	case MethodApplePay, MethodGooglePay, MethodCard, MethodSBP:
		return true
	}
	return false
}
