-- +goose Up
CREATE TABLE organizations (
    id              UUID         PRIMARY KEY DEFAULT uuidv7(),
    name            VARCHAR(255) NOT NULL,
    type            VARCHAR(20)  NOT NULL DEFAULT 'personal',
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ  NULL,

    CONSTRAINT ck_organizations_type
        CHECK (type IN ('personal', 'business'))
);

CREATE TRIGGER trg_organizations_updated_at
    BEFORE UPDATE ON organizations
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE organizations;
