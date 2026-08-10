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

ALTER TABLE users
    ADD CONSTRAINT fk_users_last_active_organization
    FOREIGN KEY (last_active_organization_id) REFERENCES organizations(id);

-- +goose Down
ALTER TABLE users DROP CONSTRAINT IF EXISTS fk_users_last_active_organization;

DROP TABLE organizations;
