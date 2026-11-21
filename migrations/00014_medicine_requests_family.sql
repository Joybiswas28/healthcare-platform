-- +goose Up
-- A patient can ask pharmacies for medicines for a family member. Delivery
-- still goes to the account holder's address and phone.
ALTER TABLE medicine_requests ADD COLUMN family_member_id UUID REFERENCES family_members (id);

-- +goose Down
ALTER TABLE medicine_requests DROP COLUMN family_member_id;
