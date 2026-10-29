export type Listing = {
  id: string;
  seller_username: string;
  external_id?: string | null;
  category: "sports" | "tcg";
  game_or_sport: string;
  set_name: string;
  year: number | null;
  card_number: string | null;
  title: string;
  description: string;
  condition: string | null;
  grader: string | null;
  grade: number | null;
  cert_number: string | null;
  attributes: string[];
  photo_urls: string[];
  price_cents: number;
  quantity: number;
  package_preset: string;
  weight_oz: number;
  status: string;
  version: number;
  created_at: string;
  updated_at: string;
  format: "fixed" | "auction";
  auction?: Auction;
  offers_enabled: boolean;
  offer_settings?: OfferSettings;
};

export type Auction = {
  start_cents: number;
  current_cents: number;
  bid_count: number;
  has_reserve: boolean;
  reserve_met: boolean;
  reserve_cents?: number; // seller only
  buy_now_cents?: number; // only while Buy It Now is available
  duration_days: number;
  starts_at: string;
  ends_at: string;
  ended_at?: string;
  state: "live" | "won" | "unsold" | "ended_early";
  auto_relist: boolean;
  round: number;
};

export type OfferSettings = {
  enabled: boolean;
  accept_kind: "amount" | "percent" | null;
  accept_value: number | null;
  decline_kind: "amount" | "percent" | null;
  decline_value: number | null;
};

export type BidHistory = {
  bids: { bidder: string; cents: number; at: string; you: boolean }[];
  bidders: number;
  leading: boolean;
  my_max_cents?: number;
  won: boolean;
  outbid: boolean;
  min_next_cents: number;
  cap_cents: number;
};

export type Offer = {
  id: string;
  listing_id: string;
  listing_title: string;
  listing_photo: string;
  price_cents: number;
  from_role: "buyer" | "seller";
  amount_cents: number;
  quantity: number;
  message: string;
  status: "pending" | "accepted" | "declined" | "countered" | "expired";
  decided_by?: "auto" | "person" | "system";
  expires_at: string;
  responded_at?: string;
  created_at: string;
  buyer: string;
  seller: string;
  can_respond: boolean;
  offers_left: number;
};

export type Hold = {
  id: string;
  listing_id: string;
  listing_title: string;
  listing_photo: string;
  source: "auction" | "offer";
  quantity: number;
  unit_cents: number;
  status: "awaiting_payment" | "paid" | "expired" | "cancelled";
  due_at: string;
  created_at: string;
  buyer: string;
  seller: string;
};

export type SearchResult = {
  listings: Listing[];
  total: number;
  limit: number;
  offset: number;
  /** Set when the API couldn't be reached, so pages don't claim "no cards". */
  failed?: boolean;
};

export type Facet = { value: string; count: number };
export type Facets = {
  category: Facet[];
  game: Facet[];
  grader: Facet[];
  condition: Facet[];
  attribute: Facet[];
  format: Facet[];
};

export type User = {
  id: string;
  email: string;
  username: string;
  role: string;
  status: string;
  email_verified: boolean;
  created_at: string;
  staff_role?: string;
};

export type Seller = {
  stripe_account_id: string;
  charges_enabled: boolean;
  details_submitted: boolean;
  tier: number;
  tier_cap_cents: number;
  bundle_discount_type: string | null;
  bundle_discount_val: number | null;
  fcm_connected: boolean;
  created_at: string;
  review_status: "pending" | "approved" | "rejected";
  collect_tax: boolean;
};

export type ApiError = {
  error: { code: string; message: string; fields?: Record<string, string> };
};

export type Summary = {
  user: User;
  is_seller: boolean;
  cart_cards: number;
  watching: number;
  unread_notices: number;
  unread_messages: number;
};

export type WatchItem = { listing: Listing; available: boolean; in_cart: boolean };

export type CartLine = { listing: Listing; quantity: number; line_cents: number; available: boolean; problem?: string };
export type SellerGroup = {
  seller_username: string;
  lines: CartLine[];
  card_count: number;
  subtotal_cents: number;
  discount_cents: number;
  discount_label?: string;
  total_cents: number;
  ship_method: "envelope" | "label";
};
export type Cart = { groups: SellerGroup[]; card_count: number; total_cents: number; problems: number };

export type Thread = {
  id: string;
  listing_id: string | null;
  listing_title: string;
  listing_photo: string;
  other_username: string;
  role: "buyer" | "seller";
  last_message: string;
  last_message_at: string;
  unread: boolean;
};
export type Message = { id: number; mine: boolean; body: string; created_at: string };

export type Notice = {
  id: string;
  kind: string;
  title: string;
  body: string;
  link: string | null;
  read: boolean;
  created_at: string;
};
export type Bell = { unread: number; notices: Notice[] };

export interface OrderLine {
  listing_id: string | null;
  title: string;
  photo: string;
  source: "cart" | "auction" | "offer";
  unit_cents: number;
  quantity: number;
}

export interface ShipTo {
  name?: string;
  line1?: string;
  line2?: string;
  city?: string;
  state?: string;
  zip?: string;
  country?: string;
}

export interface OrderFeedback {
  id: string;
  rating: -1 | 0 | 1;
  comment: string;
  item_rating: number | null;
  shipping_rating: number | null;
  communication_rating: number | null;
  state: "published" | "held" | "mediation";
  visible: boolean;
  held_until: string | null;
  reply: string | null;
  created_at: string;
}

export interface Order {
  id: string;
  number: number;
  status: "paid" | "shipped" | "delivered" | "cancelled" | "refunded" | "partially_refunded";
  buyer: string;
  seller: string;
  subtotal_cents: number;
  discount_cents: number;
  total_cents: number;
  fee_cents?: number;
  refunded_cents: number;
  ship_method: "envelope" | "label";
  ship_to: ShipTo;
  ship_by: string | null;
  created_at: string;
  paid_at: string | null;
  shipped_at: string | null;
  delivered_at: string | null;
  lines: OrderLine[];
  shipment: {
    method: string; carrier: string | null; service: string | null; tracking_number: string | null; tracking_url: string | null;
    status: string; last_event: string | null; label_url?: string; label_bought: boolean; charge_cents?: number; created_at: string;
  } | null;
  my_feedback: OrderFeedback | null;
  their_feedback: OrderFeedback | null;
  their_feedback_hidden: boolean;
  cases: { number: number; kind: string; status: string }[];
  side: "buying" | "selling";
  tax_cents: number;
  stripe_payment_url?: string;
}

export interface FeedbackSummary {
  username: string;
  score: number;
  positive_percent: number | null;
  positive_12m: number;
  neutral_12m: number;
  negative_12m: number;
  total: number;
  item_avg: number | null;
  shipping_avg: number | null;
  communication_avg: number | null;
  under_review: number;
  member_since: string;
  sales: number;
  badges: { key: string; label: string; hint: string }[];
  outside: { platform: string; profile_url: string; feedback_count: number; positive_percent: number; verified_at: string }[];
}

export interface FeedbackEntry {
  id: string;
  from: string;
  role: string;
  rating: -1 | 0 | 1;
  comment: string;
  item_rating: number | null;
  shipping_rating: number | null;
  communication_rating: number | null;
  item_title: string;
  reply: string | null;
  can_reply?: boolean;
  created_at: string;
  verified_purchase: boolean;
}
