import Link from "next/link";
import { apiGet } from "@/lib/api";
import { requireUser } from "@/lib/auth";
import { EndpointForm, NewKeyForm, RevokeKey } from "@/components/ConnectForms";

export const dynamic = "force-dynamic";
export const metadata = { title: "Connect FireCardManager", robots: { index: false } };

type Key = { id: string; name: string; prefix: string; created_at: string; last_used_at: string | null; revoked_at: string | null };
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
        <p className="mt-2 text-[14px]">Paste this into FireCardManager&apos;s marketplace settings. Anyone with the key can manage your listings, so keep it private.</p>
        {active.length > 0 && (
          <ul className="mt-3 text-[14px]">
            {active.map((k) => (
              <li key={k.id} className="flex flex-wrap items-baseline justify-between gap-2 border-b border-wash py-2">
                <span><span className="font-semibold">{k.name}</span> · <code className={code}>{k.prefix}…</code></span>
                <span className="text-muted">last used {when(k.last_used_at)} · <RevokeKey id={k.id} /></span>
              </li>
            ))}
          </ul>
        )}
        <div className="mt-3"><NewKeyForm /></div>
      </section>

      <section className="mt-8">
        <h2 className="border-b border-line pb-2 text-[18px] font-bold">2. Where to send sales</h2>
        <div className="mt-3"><EndpointForm webhook={endpoint.webhook_url} inventory={endpoint.inventory_url} hasSecret={endpoint.has_secret} /></div>
        {endpoint.reconciled_at && <p className="mt-2 text-[13px] text-muted">Inventory last checked {when(endpoint.reconciled_at)}.</p>}
      </section>

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
