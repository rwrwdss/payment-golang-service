package payments

// Method — способ, которым клиент оплачивает платёж.
type Method string

const (
	MethodCard Method = "card" // банковская карта
	MethodSBP  Method = "sbp"  // Система быстрых платежей
)

// IsValid сообщает, поддерживается ли этот способ оплаты.
func (m Method) IsValid() bool {
	switch m {
	case MethodCard, MethodSBP:
		return true
	}
	return false
}
