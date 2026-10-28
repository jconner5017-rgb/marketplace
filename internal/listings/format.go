package listings

import (
	"database/sql"
	"errors"
	"fmt"
	"slices"
	"time"
)

// Selling formats.
const (
	FormatFixed   = "fixed"
	FormatAuction = "auction"
)

var (
	AuctionDurations = []int{5, 7, 10}
	ErrHasBids       = errors.New("this auction has bids, so it can't be ended or changed")
	ErrFormatChange  = errors.New("a listing's format can't be changed; end it and list the card again")
)

// AuctionInput is what a seller sets for an auction.
type AuctionInput struct {
	StartCents   int  `json:"start_cents"`
	ReserveCents *int `json:"reserve_cents"`
	BuyNowCents  *int `json:"buy_now_cents"`
	DurationDays int  `json:"duration_days"`
	AutoRelist   bool `json:"auto_relist"`
}

// OfferSettings controls Best Offer on a fixed-price listing. Kinds are
// "amount" (value in cents) or "percent" (value = whole percent of the price).
// Offers at or above the accept threshold are accepted automatically, and
// offers below the decline threshold are declined automatically.
type OfferSettings struct {
	Enabled      bool    `json:"enabled"`
	AcceptKind   *string `json:"accept_kind"`
	AcceptValue  *int    `json:"accept_value"`
	DeclineKind  *string `json:"decline_kind"`
	DeclineValue *int    `json:"decline_value"`
}

// AuctionView is the public state of an auction. ReserveCents is filled in
// only for the seller; BuyNowCents only while Buy It Now is still available.
type AuctionView struct {
	StartCents   int        `json:"start_cents"`
	CurrentCents int        `json:"current_cents"`
	BidCount     int        `json:"bid_count"`
	HasReserve   bool       `json:"has_reserve"`
	ReserveMet   bool       `json:"reserve_met"`
	ReserveCents *int       `json:"reserve_cents,omitempty"`
	BuyNowCents  *int       `json:"buy_now_cents,omitempty"`
	DurationDays int        `json:"duration_days"`
	StartsAt     time.Time  `json:"starts_at"`
	EndsAt       time.Time  `json:"ends_at"`
	EndedAt      *time.Time `json:"ended_at,omitempty"`
	State        string     `json:"state"` // live, won, unsold, ended_early
	AutoRelist   bool       `json:"auto_relist"`
	Round        int        `json:"round"`
}

// private auction/offer fields read with every listing.
type saleFields struct {
	reserveCents *int
	binCents     *int
	leaderID     string
	winnerID     string
	offers       OfferSettings
}

// LeaderID is the current high bidder (empty if none). Never sent to clients.
func (l Listing) LeaderID() string { return l.sale.leaderID }

// WinnerID is the auction winner (empty if none).
func (l Listing) WinnerID() string { return l.sale.winnerID }

// ReserveCents is the hidden reserve price, if any.
func (l Listing) ReserveCents() *int { return l.sale.reserveCents }

// Offers returns the seller's Best Offer settings.
func (l Listing) Offers() OfferSettings { return l.sale.offers }

// Open reports whether buyers can act on the listing right now.
func (l Listing) Open() bool {
	if l.Status != "active" || l.Quantity < 1 {
		return false
	}
	if l.Format == FormatAuction {
		return l.Auction != nil && l.Auction.State == "live" && time.Now().Before(l.Auction.EndsAt)
	}
	return true
}

// ForSeller adds the fields only the seller may see.
func (l Listing) ForSeller() Listing {
	if l.Auction != nil {
		a := *l.Auction
		a.ReserveCents = l.sale.reserveCents
		l.Auction = &a
	}
	if l.Format == FormatFixed {
		o := l.sale.offers
		l.OfferSettings = &o
	}
	return l
}

// Threshold turns an accept/decline setting into cents for a given price.
func Threshold(kind *string, val *int, priceCents int) (int, bool) {
	if kind == nil || val == nil {
		return 0, false
	}
	if *kind == "percent" {
		return priceCents * *val / 100, true
	}
	return *val, true
}

const saleCols = `, l.format, l.start_cents, l.reserve_cents, l.bin_cents, l.duration_days, l.auto_relist,
	l.auction_round, l.starts_at, l.ends_at, l.ended_at, l.bid_count, l.leader_id, l.auction_state, l.winner_id,
	l.offers_enabled, l.offer_accept_kind, l.offer_accept_val, l.offer_decline_kind, l.offer_decline_val`

type saleScan struct {
	format                        string
	start, reserve, bin, duration sql.NullInt64
	autoRelist                    bool
	round                         int
	startsAt, endsAt, endedAt     sql.NullTime
	bidCount                      int
	leader, state, winner         sql.NullString
	offersOn                      bool
	acceptKind, declineKind       sql.NullString
	acceptVal, declineVal         sql.NullInt64
}

func (s *saleScan) dest() []any {
	return []any{&s.format, &s.start, &s.reserve, &s.bin, &s.duration, &s.autoRelist,
		&s.round, &s.startsAt, &s.endsAt, &s.endedAt, &s.bidCount, &s.leader, &s.state, &s.winner,
		&s.offersOn, &s.acceptKind, &s.acceptVal, &s.declineKind, &s.declineVal}
}

func nInt(n sql.NullInt64) *int {
	if !n.Valid {
		return nil
	}
	v := int(n.Int64)
	return &v
}

func nStr(n sql.NullString) *string {
	if !n.Valid {
		return nil
	}
	return &n.String
}

func (s *saleScan) apply(l *Listing) {
	l.Format = s.format
	l.sale = saleFields{reserveCents: nInt(s.reserve), binCents: nInt(s.bin),
		leaderID: s.leader.String, winnerID: s.winner.String}
	l.sale.offers = OfferSettings{Enabled: s.offersOn, AcceptKind: nStr(s.acceptKind), AcceptValue: nInt(s.acceptVal),
		DeclineKind: nStr(s.declineKind), DeclineValue: nInt(s.declineVal)}
	l.OffersEnabled = s.format == FormatFixed && s.offersOn
	if s.format != FormatAuction {
		return
	}
	a := &AuctionView{
		StartCents: int(s.start.Int64), CurrentCents: l.PriceCents, BidCount: s.bidCount,
		HasReserve: s.reserve.Valid, DurationDays: int(s.duration.Int64),
		StartsAt: s.startsAt.Time, EndsAt: s.endsAt.Time, State: s.state.String,
		AutoRelist: s.autoRelist, Round: s.round,
	}
	if s.endedAt.Valid {
		t := s.endedAt.Time
		a.EndedAt = &t
	}
	a.ReserveMet = !s.reserve.Valid || (s.bidCount > 0 && l.PriceCents >= int(s.reserve.Int64))
	if s.bidCount == 0 && a.State == "live" {
		a.BuyNowCents = nInt(s.bin)
	}
	l.Auction = a
}

// auctionFromListing rebuilds a seller's auction settings from a stored listing.
func auctionFromListing(l Listing) *AuctionInput {
	if l.Auction == nil {
		return nil
	}
	return &AuctionInput{StartCents: l.Auction.StartCents, ReserveCents: l.sale.reserveCents,
		BuyNowCents: l.sale.binCents, DurationDays: l.Auction.DurationDays, AutoRelist: l.Auction.AutoRelist}
}

// validateSale checks format, auction and offer settings.
func (in *Input) validateSale(f map[string]string, capCents int) {
	money := func(c int) string { return fmt.Sprintf("$%d.%02d", c/100, c%100) }
	switch in.Format {
	case FormatFixed:
		if in.Auction != nil {
			f["auction"] = "only auction listings have auction settings"
		}
	case FormatAuction:
		a := in.Auction
		if a == nil {
			f["auction"] = "set a starting bid and auction length"
			return
		}
		if !slices.Contains(AuctionDurations, a.DurationDays) {
			f["auction.duration_days"] = "5, 7 or 10 days"
		}
		switch {
		case a.StartCents < 1:
			f["auction.start_cents"] = "starting bid must be at least $0.01"
		case a.StartCents > capCents:
			f["auction.start_cents"] = "your seller level allows up to " + money(capCents)
		}
		if a.ReserveCents != nil {
			switch {
			case *a.ReserveCents <= a.StartCents:
				f["auction.reserve_cents"] = "reserve must be higher than the starting bid"
			case *a.ReserveCents > capCents:
				f["auction.reserve_cents"] = "your seller level allows up to " + money(capCents)
			}
		}
		if a.BuyNowCents != nil {
			low := a.StartCents
			if a.ReserveCents != nil && *a.ReserveCents > low {
				low = *a.ReserveCents
			}
			switch {
			case *a.BuyNowCents <= low:
				f["auction.buy_now_cents"] = "Buy It Now must be higher than the starting bid and reserve"
			case *a.BuyNowCents > capCents:
				f["auction.buy_now_cents"] = "your seller level allows up to " + money(capCents)
			}
		}
		if in.Status != "active" {
			f["status"] = "auctions are always active; end the auction instead"
		}
	default:
		f["format"] = "fixed or auction"
	}

	o := in.Offers
	if o == nil || !o.Enabled {
		return
	}
	if in.Format != FormatFixed {
		f["offers"] = "offers are for Buy It Now listings"
		return
	}
	check := func(field string, kind *string, val *int) (int, bool) {
		if kind == nil && val == nil {
			return 0, false
		}
		if kind == nil || val == nil || (*kind != "amount" && *kind != "percent") {
			f[field] = "choose dollars or percent and a value"
			return 0, false
		}
		if *kind == "percent" && (*val < 1 || *val > 100) {
			f[field] = "1 to 100 percent"
			return 0, false
		}
		c, _ := Threshold(kind, val, in.PriceCents)
		if c < 1 || c > in.PriceCents {
			f[field] = "must be between $0.01 and the price"
			return 0, false
		}
		return c, true
	}
	acc, hasAcc := check("offers.accept", o.AcceptKind, o.AcceptValue)
	dec, hasDec := check("offers.decline", o.DeclineKind, o.DeclineValue)
	if hasAcc && hasDec && dec >= acc {
		f["offers.decline"] = "the decline amount must be lower than the accept amount"
	}
}
