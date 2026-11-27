# Project wiki

Every project has a wiki: a tree of Markdown pages that lives next to the project board. Open a project and use the **Wiki** tab. This note describes how it is stored and how a service desk is meant to build on it.

## Where pages live

Pages are ordinary Drive files inside the project's own folder:

```
/Projects/<project>/.project.json                     the board (existing)
/Projects/<project>/wiki/<page>.json                  one page
/Projects/<project>/wiki/.history/<page>/<n>.json     earlier versions of a page
```

Because they are plain files, pages are covered by Drive sync, backups and personal share links with no extra plumbing, and they follow the same per-user scope and access rules as the rest of the project folder.

A page is a JSON document. The server only reads these fields and keeps everything else as-is:

| Field | Meaning |
| --- | --- |
| `id` | URL-safe slug, same rule as project ids (`^[a-z0-9][a-z0-9-]{0,63}$`) |
| `title` | Required, up to 200 characters |
| `parentId` | Parent page id, or `""` for a top-level page |
| `version`, `updatedAt` | Set by the server on every save |
| `body` | Markdown source, up to 1 MiB |
| `refs` | Ids of board cards (parent items) the page documents |
| `kind` | `"page"` today; reserved for other page types (see below) |

## API

All routes need a signed-in user and act on that user's own `/Projects` folder.

| Route | Purpose |
| --- | --- |
| `GET /api/projects/{id}/wiki` | List pages. Bodies are left out so the tree stays cheap. |
| `GET /api/projects/{id}/wiki/{page}` | One page, with body. |
| `PUT /api/projects/{id}/wiki/{page}` | Create (send `version: 0`) or update. |
| `DELETE /api/projects/{id}/wiki/{page}` | Delete a page and its history. |
| `GET /api/projects/{id}/wiki/{page}/history` | Saved versions, newest first. |
| `GET /api/projects/{id}/wiki/{page}/history/{n}` | One saved version, with body. |

Behaviour worth knowing:

- **Conflicts.** A save carries the version the client last saw. If someone saved first the server returns `409` instead of overwriting, exactly as projects do.
- **History.** Each save keeps the version it replaced. The newest 50 are kept.
- **Tree.** A parent must exist and cannot be the page itself or one of its descendants. A page that still has children cannot be deleted.
- **Permissions.** Creating needs the create permission, editing needs modify, deleting needs delete. The project must already exist.

## Linking

- `[[Page title]]`, `[[page-id]]` and `[[page-id|label]]` link to another page in the same project. A target that matches nothing is shown in red so a broken link is easy to spot. Links inside code spans and code blocks are left alone.
- A page can be linked to board cards while editing. Those pages then appear on the card in the **Board** tab, and the card's name appears on the page.
- A project created from an imported ticket starts its home page from the ticket's details.
- `?tab=wiki&page=<id>` opens a specific page, so pages can be bookmarked or linked to.

## Building the service desk on this

The service desk should reuse this storage rather than add a second system:

1. **Knowledge-base articles** are pages with `kind: "article"` in a project (or a dedicated "Support" project). The same editor, history, links and sync apply. The list endpoint can gain a `?kind=` filter, and the tree can hide articles from the normal wiki view.
2. **Requests** (tickets raised by customers) are a new document type next to the board, for example `/Projects/<project>/desk/<request>.json`, copied from the project and wiki handlers: slug id, `version` conflict check, envelope fields, opaque remainder. Requests can carry `refs` to wiki articles ("suggested article") and to board cards ("work in progress"), mirroring how pages already reference cards.
3. **Seamless sync between the three** comes from every document holding stable ids of the others, not from copying content. A request resolves its article and card by id when displayed, so edits to either show up immediately.
4. **Requester access** is the one genuinely new piece. Everything today is scoped to the signed-in owner, so letting outside requesters file and read requests needs its own authentication and a narrow, separate route, designed deliberately instead of loosening the project routes.

The existing ticket import (`frontend/src/utils/ticketImport.ts`) already turns a hub ticket into a project, which is the natural bridge until native requests exist.
