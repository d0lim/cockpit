CREATE TABLE items (
  id SERIAL PRIMARY KEY,
  name TEXT NOT NULL,
  price_cents INTEGER NOT NULL
);

INSERT INTO items (name, price_cents) VALUES ('kettle', 2500), ('teapot', 1800), ('mug', 600);
