-- +goose Up
CREATE TABLE library_items (
    id          BIGINT GENERATED ALWAYS AS IDENTITY PRIMARY KEY,
    user_id     INT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    content_id  BIGINT NOT NULL REFERENCES article_contents(id),
    state       TEXT NOT NULL DEFAULT 'unread'
                CHECK (state IN ('unread', 'read', 'archived')),
    is_favorite BOOLEAN NOT NULL DEFAULT FALSE,
    saved_at    TIMESTAMPTZ NOT NULL DEFAULT now(),
    read_at     TIMESTAMPTZ,
    UNIQUE (user_id, content_id)
);

CREATE INDEX library_items_user_saved_idx ON library_items (user_id, saved_at DESC);
CREATE INDEX library_items_user_state_idx ON library_items (user_id, state);

-- +goose Down
DROP TABLE library_items;
