-- Store sale % off rounds to the nearest cent (was always down), so a 15%
-- sale on $1.88 takes 28¢ off and a 15% sale on $0.99 takes 15¢, not 14¢.
CREATE OR REPLACE FUNCTION store_sale_price(p_seller UUID, p_category INT, p_game TEXT, p_price INT) RETURNS INT
LANGUAGE sql STABLE AS $$
    SELECT coalesce(min(greatest(
               p_price - CASE WHEN ss.amount_cents IS NOT NULL THEN ss.amount_cents
                              ELSE round(p_price * ss.percent / 100.0)::int END,
               p_price - p_price / 2)), p_price)
    FROM store_sales ss JOIN stores st ON st.seller_id = ss.seller_id AND st.status = 'active'
    WHERE ss.seller_id = p_seller AND ss.cancelled_at IS NULL
      AND now() >= ss.starts_at AND now() < ss.ends_at
      AND (ss.category_id IS NULL OR ss.category_id = p_category)
      AND (ss.games IS NULL OR upper(p_game) = ANY (SELECT upper(g) FROM unnest(ss.games) g))
$$;
