-- +goose Up
CREATE TABLE organization_invitations (
    id              UUID         PRIMARY KEY DEFAULT uuidv7(),
    organization_id UUID         NOT NULL REFERENCES organizations(id),
    email           VARCHAR(255) NOT NULL,
    role            VARCHAR(20)  NOT NULL,
    token_hash      VARCHAR(255) NOT NULL,
    invited_by      UUID         NOT NULL REFERENCES users(id),
    status          VARCHAR(20)  NOT NULL DEFAULT 'pending',
    expires_at      TIMESTAMPTZ  NOT NULL,
    accepted_at     TIMESTAMPTZ  NULL,
    accepted_by     UUID         NULL REFERENCES users(id),
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT ck_org_invitations_role
        CHECK (role IN ('owner', 'admin', 'member')),
    CONSTRAINT ck_org_invitations_status
        CHECK (status IN ('pending', 'accepted', 'expired', 'revoked'))
);

CREATE UNIQUE INDEX ux_org_invitations_token
    ON organization_invitations (token_hash);

CREATE UNIQUE INDEX ux_org_invitations_pending
    ON organization_invitations (organization_id, lower(email))
    WHERE status = 'pending';

CREATE INDEX ix_org_invitations_email
    ON organization_invitations (lower(email)) WHERE status = 'pending';

-- +goose Down
DROP TABLE organization_invitations;
