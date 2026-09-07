-- +goose Up
CREATE TABLE organization_service_access
(
    id              UUID         PRIMARY KEY DEFAULT uuidv7(),
    organization_id UUID         NOT NULL,
    user_id         UUID         NOT NULL,
    service_name    VARCHAR(50)  NOT NULL,
    granted_by      UUID         NOT NULL,
    created_at      TIMESTAMPTZ  NOT NULL DEFAULT now(),

    CONSTRAINT fk_service_access_granted_by FOREIGN KEY (granted_by) REFERENCES users(id)
);

CREATE UNIQUE INDEX ux_service_access_org_user_service
    ON organization_service_access (organization_id, user_id, service_name);

CREATE INDEX ix_service_access_org_user
    ON organization_service_access (organization_id, user_id);

CREATE INDEX ix_service_access_user_service
    ON organization_service_access (user_id, service_name);

-- +goose Down
DROP TABLE IF EXISTS organization_service_access;
