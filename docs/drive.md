# Drive: modern UI, custom share links, and Supabase auth

This fork restyles File Browser as a modern cloud-storage app ("Drive") and adds a few features on top of the upstream code.

## What's new

- **Modern UI.** New sidebar with a "New" menu (folder, file, file upload, folder upload), a storage meter, a user menu with a Light/Dark/Auto theme switch, list/grid/gallery views, an empty state, a drop overlay for uploads, and a responsive drawer on mobile.
- **Personal share links.** When you share, you can pick your own slug, such as `/share/q4-report`, instead of a random hash. The dialog checks availability while you type. Leave the slug blank to get a random link as before. It also includes expiry presets, an optional password, auto-copy, and a QR code.
- **Starred.** You can star files and folders from the toolbar, the context menu, or the mobile action bar. Stars are saved per user.
- **Recent.** Shows recently modified files across your scope, grouped by date.
- **Shared links.** One page that lists every link you've shared.
- **Supabase sign-in.** Sign in with email and password, OAuth providers (Google, GitHub, and others), or a magic link, all through your own Supabase/GoTrue instance.

## Supabase setup

1. Set the auth method (stop the server first, or run against a copy of the database):

   ```sh
   filebrowser config set \
     --auth.method=supabase \
     --auth.supabaseUrl=https://base.millitop.net \
     --auth.supabaseAnonKey=<ANON_KEY> \
     --auth.supabaseAdmins=you@example.com \
     --auth.supabaseProviders=google \
     --auth.supabaseMagicLink
   ```

   Optional flags:

   | Flag | Default | Purpose |
   | --- | --- | --- |
   | `--auth.supabaseAllowedDomains=example.com,other.com` | (any) | Only these email domains can sign in |
   | `--auth.supabaseAllowLocal` | `true` | Keeps local File Browser accounts working as a break-glass login. Set it to `false` once Supabase sign-in works. |
   | `--auth.supabaseAdmins` | (none) | Comma-separated emails that get admin |

   Each flag can also be set as an environment variable with the `FB_` prefix.

2. In Supabase/GoTrue, add the Drive login URL to the redirect allow-list. On self-hosted Docker this is `ADDITIONAL_REDIRECT_URLS` (`GOTRUE_URI_ALLOW_LIST`):

   ```
   https://<drive-host>/login
   ```

   If you serve Drive under a base URL, include it, e.g. `https://host/drive/login`.

3. Enable the OAuth providers you listed in GoTrue (for example `GOTRUE_EXTERNAL_GOOGLE_ENABLED=true` plus client ID and secret). For magic links, SMTP must be configured.

4. Restart Drive. The login page now shows the provider buttons and an "Email me a link instead" option.

### How users and admins map

- The first time someone signs in with Supabase, Drive creates a matching user keyed by their lowercased email. New users get the default user settings.
- A user is an admin if their email is in `auth.supabaseAdmins`, **or** their Supabase `app_metadata.drive_admin` is `true`.
- Magic links are sent with `create_user: false`, so the user must already exist in Supabase. Drive won't create Supabase accounts on its own.
- OAuth and magic-link logins come back to `/login#access_token=...`. Drive checks the token against `GET /auth/v1/user` and then issues its own session.

## Custom share slugs

- Allowed format: `^[a-z0-9][a-z0-9_-]{2,63}$`. Input is lowercased, and spaces become `-`.
- A slug that's already taken returns `409`. An invalid slug returns `400`.
- The public URL is `/share/<slug>`. Downloads and passwords work the same as random links.

## New API endpoints

| Method | Path | Notes |
| --- | --- | --- |
| `POST` | `/api/auth/magic-link` | `{ "email": "..." }` |
| `GET` | `/api/share-slug/{slug}` | Availability check |
| `GET` | `/api/starred` | Starred entries `{path,name,size,modified,isDir}` |
| `PUT` | `/api/starred` | `{ "path": "/x", "starred": true }` returns the new list of paths |
| `GET` | `/api/recent?limit=50` | `{ items, partial }` |
| `POST` | `/api/share/{path}` | Now accepts an optional `slug` field |
