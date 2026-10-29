-- Copyright 2025 β ORI Inc. Canada. All Rights Reserved.
-- Author: Awase Khirni Syed
--
-- 000001_create_users.up.sql - reference table for the User aggregate.
-- Schema-as-code: every change goes through a migration file, never through
-- a manual ALTER on a live DB.

CREATE EXTENSION IF NOT EXISTS "uuid-ossp";

CREATE TABLE IF NOT EXISTS users (
    id           UUID         PRIMARY KEY DEFAULT uuid_generate_v4(),
    email        VARCHAR(255) NOT NULL UNIQUE,
    first_name   VARCHAR(100) NOT NULL,
    last_name    VARCHAR(100) NOT NULL,
    status       VARCHAR(20)  NOT NULL DEFAULT 'pending'
                              CHECK (status IN ('active','inactive','pending')),
    is_deleted   BOOLEAN      NOT NULL DEFAULT false,
    deleted_at   TIMESTAMPTZ,
    created_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    updated_at   TIMESTAMPTZ  NOT NULL DEFAULT now(),
    version      INTEGER      NOT NULL DEFAULT 1
);

-- Soft-delete filter helper index (queries use `WHERE is_deleted=false`).
CREATE INDEX IF NOT EXISTS idx_users_is_deleted ON users(is_deleted) WHERE is_deleted = false;

-- Status filter index.
CREATE INDEX IF NOT EXISTS idx_users_status ON users(status) WHERE is_deleted = false;

-- Pagination cursor index.
CREATE INDEX IF NOT EXISTS idx_users_created_at ON users(created_at DESC);

-- Audit trigger: bump updated_at on every UPDATE.
CREATE OR REPLACE FUNCTION bump_updated_at() RETURNS TRIGGER AS $$
BEGIN
    NEW.updated_at = now();
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_users_bump_updated_at ON users;
CREATE TRIGGER trg_users_bump_updated_at BEFORE UPDATE ON users
    FOR EACH ROW EXECUTE FUNCTION bump_updated_at();
