import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { EndpointForm, NewKeyForm, RevokeKey } from "@/components/ConnectForms";

export const dynamic = "force-dynamic";
export const metadata = { title: "Connect FireCardManager", robots: { index: false } };

type Key = { id: string; name: string; prefix: string; kind: "desktop" | "web"; created_at: string; last_used_at: string | null; revoked_at: string | null };
type Delivery = { id: number; event: string; attempts: number; delivered_at: string | null; last_status: number | null; last_error: string | null; gave_up_at: string | null; next_attempt_at: string; created_at: string };
type Connect = { keys: Key[]; endpoint: { webhook_url: string; inventory_url: string; has_secret: boolean; reconciled_at: string | null }; deliveries: Delivery[] };

const when = (iso: string | null) => (iso ? new Date(iso).toLocaleString("en-US", { month: "short", day: "numeric", hour: "numeric", minute: "2-digit" }) : "—");

export default async function ConnectPage() {
  await requireUser("/sell/connect");
  const r = await apiGet<Connect>("/v1/seller/connect");
  if (!r.data) {
    return <p className="py-16 text-center">Set up selling first. <Link href="/sell" className="text-link underline">Seller hub</Link></p>;
  }
  const { keys, endpoint, deliveries } = r.data;
  const active = keys.filter((k) => !k.revoked_at);
  const code = "rounded bg-wash px-1 py-0.5 text-[13px]";
  return (
    <div className="max-w-[820px] pt-6">
      <Link href="/sell" className="text-[14px] text-link hover:underline">← Seller hub</Link>
      <h1 className="mt-2 text-[26px] font-bold">Connect FireCardManager</h1>
      <p className="text-muted">
        Let your inventory tool list cards here and hear about sales the moment they&apos;re paid, so the same card is taken off
        eBay and your other sites. It&apos;s free.
      </p>

      <section className="mt-6">
        <h2 className="border-b border-line pb-2 text-[18px] font-bold">1. API key</h2>
        <p className="mt-2 text-[14px]">
          Create a key, then paste it into FireCardManager under <strong>Settings → Slabs &amp; Singles</strong>. That&apos;s all the
          desktop app needs: it lists your cards, checks for sales every few minutes while it&apos;s open, ends the eBay listing when a
          card sells here, and sends your tracking number back. Anyone with the key can manage your listings, so keep it private.
        </p>
        <p className="mt-2 text-[14px]">
          Using <strong>FireCardManager Web</strong> too? Make a separate key <em>for FireCardManager Web</em> and paste it on its Account page,
          so each can be revoked on its own. Only the desktop app&apos;s check-ins count for the 3-day pause below, because it&apos;s what takes
          sold cards off eBay.
        </p>
        {active.length > 0 && (
          <ul className="mt-3 text-[14px]">
            {active.map((k) => (
              <li key={k.id} className="flex flex-wrap items-baseline justify-between gap-2 border-b border-wash py-2">
                <span>
                  <span className="font-semibold">{k.name}</span>{" "}
                  <span className="rounded-full border border-line px-2 py-0.5 text-[11px] text-muted">{k.kind === "web" ? "FireCardManager Web" : "Desktop app"}</span>{" "}
                  · <code className={code}>{k.prefix}…</code>
                </span>
                <span className="text-muted">last used {when(k.last_used_at)} · <RevokeKey id={k.id} /></span>
              </li>
            ))}
          </ul>
        )}
        <div className="mt-3"><NewKeyForm /></div>
        <div className="mt-4 rounded-lg border border-slab/40 bg-[#fff6f6] p-4 text-[14px]">
          <p className="font-semibold">Keep FireCardManager running and licensed</p>
          <ul className="mt-2 list-disc space-y-1 pl-5">
            <li>FireCardManager only syncs while it&apos;s open. If it stops (trial or license ends, computer off), your eBay and Slabs &amp; Singles listings <strong>stay live</strong>, and a card could sell in both places.</li>
            <li>To protect you, we pause the cards FireCardManager listed here after <strong>3 days without a sync</strong> and email you. They come back on its next sync.</li>
            <li>eBay listings aren&apos;t paused: end them on eBay if FireCardManager will be off for a while.</li>
            <li>New sellers can list cards up to <strong>$20 each</strong>; the limit rises to $250, then $1,000, as you build sales and feedback. Pricier cards stay on eBay until then, and FireCardManager tells you which.</li>
          </ul>
        </div>
      </section>

      <details className="mt-8 rounded-lg border border-line p-4" open={!!(endpoint.webhook_url || endpoint.inventory_url)}>
        <summary className="cursor-pointer text-[16px] font-bold">Web-based inventory tools (optional)</summary>
        <p className="mt-2 text-[14px] text-muted">
          Only for tools that run on a web server and can receive calls from us. FireCardManager doesn&apos;t need these; leave them blank.
        </p>
        <div className="mt-3"><EndpointForm webhook={endpoint.webhook_url} inventory={endpoint.inventory_url} hasSecret={endpoint.has_secret} /></div>
        {endpoint.reconciled_at && <p className="mt-2 text-[13px] text-muted">Inventory last checked {when(endpoint.reconciled_at)}.</p>}
      </details>

      {deliveries.length > 0 && (
        <section className="mt-8">
          <h2 className="border-b border-line pb-2 text-[18px] font-bold">Recent events</h2>
          <div className="overflow-x-auto">
            <table className="mt-2 w-full min-w-[560px] text-[13px]">
              <thead><tr className="border-b border-line text-left text-muted"><th className="py-1.5 pr-3">Sent</th><th className="py-1.5 pr-3">Event</th><th className="py-1.5 pr-3">Result</th><th className="py-1.5">Tries</th></tr></thead>
              <tbody>
                {deliveries.map((d) => (
                  <tr key={d.id} className="border-b border-wash">
                    <td className="py-1.5 pr-3 whitespace-nowrap">{when(d.created_at)}</td>
                    <td className="py-1.5 pr-3 font-mono">{d.event}</td>
                    <td className={`py-1.5 pr-3 ${d.delivered_at ? "text-ship" : "text-slab"}`}>
                      {d.delivered_at ? `Delivered (${d.last_status})` : d.gave_up_at ? "Gave up after 24 hours" : d.attempts === 0 ? "Queued" : `Failing: ${d.last_error ?? ""}; next try ${when(d.next_attempt_at)}`}
                    </td>
                    <td className="py-1.5">{d.attempts}</td>
                  </tr>
                ))}
              </tbody>
            </table>
          </div>
        </section>
      )}

      <section className="mt-10 text-[14px]">
        <h2 className="border-b border-line pb-2 text-[18px] font-bold">For developers</h2>
        <p className="mt-2">Send <code className={code}>Authorization: Bearer &lt;API key&gt;</code> and JSON. Add an <code className={code}>Idempotency-Key</code> header to writes so a retry never duplicates anything.</p>
        <ul className="mt-2 list-disc space-y-1 pl-5">
          <li><code className={code}>POST /api/v1/listings</code> create or replace by <code className={code}>external_id</code> (your SKU)</li>
          <li><code className={code}>POST /api/v1/listings/bulk</code> up to 500 at once: <code className={code}>{"{\"listings\": [...]}"}</code></li>
          <li><code className={code}>PATCH /api/v1/listings/&#123;external_id&#125;</code> change price, quantity, photos</li>
          <li><code className={code}>DELETE /api/v1/listings/&#123;external_id&#125;</code> it sold elsewhere</li>
          <li><code className={code}>GET /api/v1/orders</code> your sales, with each card&apos;s <code className={code}>external_id</code></li>
          <li><code className={code}>POST /api/v1/orders/&#123;id&#125;/ship</code> with <code className={code}>carrier</code> and <code className={code}>tracking_number</code></li>
          <li><code className={code}>POST /api/v1/photos</code> with a JPEG/PNG as the body; returns a URL for <code className={code}>photo_urls</code></li>
          <li><code className={code}>POST /api/v1/orders/&#123;id&#125;/oversold</code> with <code className={code}>external_id</code> and <code className={code}>sold_elsewhere_at</code> when a card sold in both places; the later sale is refunded</li>
        </ul>
        <p className="mt-3">
          Webhooks are signed: <code className={code}>SS-Signature: t=&lt;unix&gt;,v1=&lt;hex&gt;</code>, where v1 is HMAC-SHA256 of
          <code className={code}>t + &quot;.&quot; + body</code> with your signing secret. Answer 2xx; failures retry with backoff for 24 hours.
          The inventory URL is called nightly with the same signature over an empty body and should answer
          <code className={code}>{"{\"items\": [{\"external_id\": \"…\", \"quantity\": 0}]}"}</code>; a quantity of 0 takes the listing down here.
        </p>
      </section>
    </div>
  );
}
