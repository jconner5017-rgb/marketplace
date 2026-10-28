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
