ALTER TABLE txouts DROP COLUMN addresses;
ALTER TABLE txouts ADD COLUMN pk_script BYTEA;
