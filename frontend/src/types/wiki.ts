/**
 * A wiki page belongs to one project and is stored in that project's Drive
 * folder. `body` is Markdown and is only present once the page has been
 * opened: the list endpoint leaves it out to keep the page tree cheap.
 */
export interface WikiPage {
  id: string;
  title: string;
  /** Id of the parent page, or "" for a top-level page. */
  parentId: string;
  version: number;
  updatedAt?: string;
  /**
   * What the page is. Only "page" exists today; the field is reserved so the
   * service desk can store knowledge-base articles with the same storage,
   * history and API instead of a second system.
   */
  kind?: string;
  /** Ids of the project's parent items (board cards) this page documents. */
  refs: string[];
  body?: string;
}

export interface WikiRevision {
  version: number;
  title: string;
  updatedAt: string;
}

export interface WikiNode {
  page: WikiPage;
  depth: number;
}
