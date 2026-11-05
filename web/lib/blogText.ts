// The back office edits a post's body as plain text, converted to and from
// blocks:
//   ## Heading
//   A paragraph. Lines next to each other join into one paragraph.
//   - a list item (lines in a row make one list)
//   > a highlighted note
//   [Link label](/help/fees) optional text after the link
import type { Block } from "./help";

type Link = { label: string; href: string; text?: string };

export function blocksToText(body: Block[]): string {
  return body.map((b) => {
    if ("h" in b) return `## ${b.h}`;
    if ("list" in b) return b.list.map((x) => `- ${x}`).join("\n");
    if ("note" in b) return `> ${b.note}`;
    if ("links" in b) return b.links.map((l) => `[${l.label}](${l.href})${l.text ? " " + l.text : ""}`).join("\n");
    return (b as { p: string }).p;
  }).join("\n\n");
}

const LINK = /^\[([^\]]+)\]\((\S+)\)\s*(.*)$/;

export function textToBlocks(text: string): Block[] {
  const out: Block[] = [];
  let para: string[] = [];
  let list: string[] = [];
  let links: Link[] = [];
  const flush = () => {
    if (para.length) out.push({ p: para.join(" ") });
    if (list.length) out.push({ list });
    if (links.length) out.push({ links });
    para = []; list = []; links = [];
  };
  for (const raw of text.replace(/\r\n/g, "\n").split("\n")) {
    const line = raw.trim();
    if (!line) { flush(); continue; }
    if (line.startsWith("## ")) { flush(); out.push({ h: line.slice(3).trim() }); continue; }
    if (line.startsWith("> ")) { flush(); out.push({ note: line.slice(2).trim() }); continue; }
    if (/^[-*•] /.test(line)) {
      if (para.length || links.length) { const l = list; list = []; flush(); list = l; }
      list.push(line.slice(2).trim());
      continue;
    }
    const m = LINK.exec(line);
    if (m) {
      if (para.length || list.length) { const k = links; links = []; flush(); links = k; }
      links.push(m[3] ? { label: m[1], href: m[2], text: m[3].replace(/^[—:-]\s*/, "") } : { label: m[1], href: m[2] });
      continue;
    }
    if (list.length || links.length) { const p = para; para = []; flush(); para = p; }
    para.push(line);
  }
  flush();
  return out;
}
