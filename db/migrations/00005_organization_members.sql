-- +goose Up
CREATE TABLE organization_members (
    organization_id UUID        NOT NULL REFERENCES organizations(id),
    user_id         UUID        NOT NULL REFERENCES users(id),
    role            VARCHAR(20) NOT NULL,
    joined_at       TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at      TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at      TIMESTAMPTZ NULL,

    PRIMARY KEY (organization_id, user_id),

    CONSTRAINT ck_organization_members_role
        CHECK (role IN ('owner', 'admin', 'member'))
);

CREATE INDEX ix_org_members_user
    ON organization_members (user_id) WHERE deleted_at IS NULL;

CREATE INDEX ix_org_members_org
    ON organization_members (organization_id) WHERE deleted_at IS NULL;

CREATE TRIGGER trg_org_members_updated_at
    BEFORE UPDATE ON organization_members
    FOR EACH ROW EXECUTE FUNCTION set_updated_at();

-- +goose Down
DROP TABLE organization_members;
