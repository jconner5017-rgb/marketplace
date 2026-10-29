-- Test orders: placed by an admin while real checkout is switched off, marked
-- paid without taking any payment. Kept apart so they can be removed before
-- launch and never mistaken for sales.
ALTER TABLE orders ADD COLUMN test BOOLEAN NOT NULL DEFAULT false;
CREATE INDEX orders_test_idx ON orders (created_at) WHERE test;
