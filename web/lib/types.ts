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
};

export type SearchResult = {
  listings: Listing[];
  total: number;
  limit: number;
  offset: number;
};

export type Facet = { value: string; count: number };
export type Facets = {
  category: Facet[];
  game: Facet[];
  grader: Facet[];
  condition: Facet[];
  attribute: Facet[];
};

export type User = {
  id: string;
  email: string;
  username: string;
  role: string;
  status: string;
  email_verified: boolean;
  created_at: string;
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
