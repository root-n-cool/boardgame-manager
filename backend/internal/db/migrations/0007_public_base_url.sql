-- The address the club reaches this instance at. It exists because a link that
-- leaves the app — today the admin invite — cannot be built from the browser's
-- own origin when the person generating it is not on the public address (behind
-- a proxy, or on localhost while the club uses a domain). Empty means "not
-- configured": callers then fall back to the origin they are already on, which
-- is what keeps a local install working with no settings at all.
ALTER TABLE app_settings ADD COLUMN public_base_url TEXT;

-- No code has ever read these three. The automatic enrichment they were meant
-- for (YouTube tutorials, manual lookup through a search API) was never built,
-- so until it is they are dead configuration that whoever installs the app has
-- to be told to ignore.
ALTER TABLE app_settings DROP COLUMN youtube_api_key;
ALTER TABLE app_settings DROP COLUMN search_api_key;
ALTER TABLE app_settings DROP COLUMN search_api_provider;
