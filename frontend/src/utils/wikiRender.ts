import DOMPurify from "dompurify";
import { marked } from "marked";
import type { WikiPage } from "@/types/wiki";
import { resolveWikiLinks } from "./wikiLogic";

/**
 * Renders a page's Markdown to sanitised HTML. Wiki links are resolved against
 * the project's other pages first; DOMPurify then strips anything unsafe, so
 * page content is never trusted even though only the project owner writes it.
 */
export function renderWiki(md: string, pages: WikiPage[]): string {
  const html = marked.parse(resolveWikiLinks(md, pages), { async: false }) as string;
  return DOMPurify.sanitize(html);
}
