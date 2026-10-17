package main

import (
	"fmt"
	"log"

	"payment-service/payments"
)

func main() {
	p, err := payments.NewPayment(15000, payments.CurrencyUSD, payments.MethodCard)
	if err != nil {
		log.Fatal(err)
	}

	fmt.Printf("payment %s: %d %s, status %s\n", p.ID, p.Amount, p.Currency, p.Status)
}
