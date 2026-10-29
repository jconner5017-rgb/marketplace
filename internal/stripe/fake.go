package stripe

import (
	"context"
	"errors"
	"fmt"
	"sync"
)

// FakePayments stands in for Stripe Checkout in tests and local development.
type FakePayments struct {
	mu       sync.Mutex
	n        int
	sessions map[string]*CheckoutSession
	accounts map[string]string
	params   map[string]CheckoutParams
	Fail     error // returned by CreateCheckoutSession when set
}

func NewFakePayments() *FakePayments {
	return &FakePayments{sessions: map[string]*CheckoutSession{}, accounts: map[string]string{}, params: map[string]CheckoutParams{}}
}

func (f *FakePayments) CreateCheckoutSession(_ context.Context, account string, p CheckoutParams) (CheckoutSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.Fail != nil {
		return CheckoutSession{}, f.Fail
	}
	if account == "" {
		return CheckoutSession{}, errors.New("fake stripe: no account")
	}
	f.n++
	id := fmt.Sprintf("cs_test_fake%d", f.n)
	total := 0
	for _, l := range p.Lines {
		total += l.UnitCents * l.Quantity
	}
	if p.AutomaticTax {
		total += total * 825 / 10000 // a stand-in 8.25% sales tax
	}
	s := &CheckoutSession{ID: id, URL: "https://checkout.stripe.test/" + id, Status: "open", PaymentStatus: "unpaid",
		AmountTotal: total, ClientReference: p.OrderID}
	f.sessions[id], f.accounts[id], f.params[id] = s, account, p
	return *s, nil
}

func (f *FakePayments) GetCheckoutSession(_ context.Context, account, id string) (CheckoutSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok || f.accounts[id] != account {
		return CheckoutSession{}, &Error{Status: 404, Message: "No such checkout.session"}
	}
	return *s, nil
}

// Params returns what a session was created with.
func (f *FakePayments) Params(id string) CheckoutParams {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.params[id]
}

// Complete marks a session paid, as if the buyer finished on Stripe's page.
func (f *FakePayments) Complete(id string) CheckoutSession {
	f.mu.Lock()
	defer f.mu.Unlock()
	s := f.sessions[id]
	s.Status, s.PaymentStatus, s.PaymentIntent = "complete", "paid", "pi_"+id
	s.ShippingDetails = &struct {
		Name    string  `json:"name"`
		Address Address `json:"address"`
	}{Name: "Pat Buyer", Address: Address{Line1: "1 Main St", City: "Austin", State: "TX", PostalCode: "78701", Country: "US"}}
	return *s
}

func (f *FakePayments) ExpireCheckoutSession(_ context.Context, account, id string) (CheckoutSession, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	s, ok := f.sessions[id]
	if !ok || f.accounts[id] != account {
		return CheckoutSession{}, &Error{Status: 404, Message: "No such checkout.session"}
	}
	if s.Status != "open" {
		return CheckoutSession{}, &Error{Status: 400, Message: "Only open sessions can be expired"}
	}
	s.Status = "expired"
	return *s, nil
}

// Expire ends a session unpaid.
func (f *FakePayments) Expire(id string) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.sessions[id].Status = "expired"
}
