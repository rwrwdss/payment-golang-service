package practic

import (
	"fmt"
	"payment-service/payments"
)

func main() {
	newOrder := payments.NewPayment{}
	fmt.Println(newOrder)
}
