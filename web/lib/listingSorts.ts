/** Seller hub listing sort orders (value, label). Shared by the server page and the client dropdown. */
export const LISTING_SORTS = [
  ["newest", "Newly listed"],
  ["oldest", "Oldest listed"],
  ["price_desc", "Price: high to low"],
  ["price_asc", "Price: low to high"],
] as const;
