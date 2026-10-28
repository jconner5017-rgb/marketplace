import { apiGet } from "@/lib/api";
import { requireStaff } from "@/lib/staff";
import ActionForm from "@/components/admin/ActionForm";
import DeleteButton from "@/components/admin/DeleteButton";
import AddFeatureForm from "@/components/admin/AddFeatureForm";

export const dynamic = "force-dynamic";
export const metadata = { title: "Categories and features" };

type Category = { id: number; kind: string; value: string; label: string; short_label: string; aliases: string[]; active: boolean; sort: number; listings: number };
type Feature = { key: string; label: string; active: boolean; sort: number; listings: number };

const aliases = (v: unknown) => String(v ?? "").split(",").map((a) => a.trim()).filter(Boolean);

export default async function CatalogPage() {
  const s = await requireStaff("/admin/catalog", "users.view");
  const edit = s.permissions.includes("catalog.edit");
  const r = await apiGet<{ categories: Category[]; features: Feature[] }>("/v1/admin/catalog");
  const cats = r.data?.categories ?? [];
  const feats = r.data?.features ?? [];
  return (
    <div>
      <h1 className="text-[26px] font-bold">Categories and features</h1>
      <p className="text-muted">
        The sports, games and card features sellers choose from and buyers filter by. Changes reach the site within 5 minutes.
        Hide something to stop new listings using it; delete only works when no listing uses it.
      </p>
      {!edit && <p className="mt-2 text-[13px] text-muted">Only a Super Admin can make changes.</p>}

      {(["sports", "tcg"] as const).map((kind) => (
        <section key={kind} className="mt-8">
          <h2 className="text-[20px] font-bold">{kind === "sports" ? "Sports" : "Trading card games"}</h2>
          <ul className="mt-3 divide-y divide-line border-y border-line">
            {cats.filter((c) => c.kind === kind).map((c) => (
              <li key={c.id} className="py-3 text-[14px]">
                <details>
                  <summary className="flex cursor-pointer list-none flex-wrap items-center gap-2 [&::-webkit-details-marker]:hidden">
                    <span className={`font-semibold ${c.active ? "" : "text-muted line-through"}`}>{c.label}</span>
                    <span className="text-[12px] text-muted">stored as “{c.value}” · {c.listings} listings{c.aliases.length ? ` · ${c.aliases.length} alternate names` : ""}</span>
                    {!c.active && <span className="rounded-full bg-wash px-2 text-[12px]">hidden</span>}
                    {edit && <span className="ml-auto text-[13px] text-link underline">Edit</span>}
                  </summary>
                  {edit && (
                    <div className="mt-3 grid gap-4 md:grid-cols-[minmax(0,1fr)_auto]">
                      <ActionForm
                        endpoint={`/v1/admin/catalog/categories/${c.id}`}
                        method="PATCH"
                        fields={[
                          { name: "label", label: "Name buyers see", initial: c.label },
                          { name: "short_label", label: "Short name (menus and tiles)", initial: c.short_label },
                          { name: "aliases", label: "Alternate names sellers might type (comma separated)", initial: c.aliases.join(", ") },
                          { name: "sort", label: "Order (lower comes first)", initial: String(c.sort) },
                          { name: "active", label: "Shown and available for new listings", type: "checkbox", initial: c.active },
                        ]}
                        transform={(v) => ({ ...v, aliases: aliases(v.aliases), sort: Number(v.sort) || 0 })}
                        submit="Save"
                        compact
                      />
                      {c.listings === 0 && <div><DeleteButton endpoint={`/v1/admin/catalog/categories/${c.id}`} /></div>}
                    </div>
                  )}
                </details>
              </li>
            ))}
          </ul>
          {edit && (
            <details className="mt-3 max-w-[520px] rounded-lg border border-line p-4">
              <summary className="cursor-pointer font-semibold">Add a {kind === "sports" ? "sport" : "game"}</summary>
              <div className="mt-3">
                <ActionForm
                  endpoint="/v1/admin/catalog/categories"
                  fields={[
                    { name: "value", label: "Stored name (can't change later)", placeholder: kind === "sports" ? "Lacrosse" : "Altered" },
                    { name: "label", label: "Name buyers see", placeholder: kind === "sports" ? "Lacrosse" : "Altered TCG" },
                    { name: "short_label", label: "Short name" },
                    { name: "aliases", label: "Alternate names (comma separated)" },
                  ]}
                  extra={{ kind }}
                  transform={(v) => ({ ...v, aliases: aliases(v.aliases) })}
                  submit="Add"
                  done="Added."
                />
              </div>
            </details>
          )}
        </section>
      ))}

      <section className="mt-10">
        <h2 className="text-[20px] font-bold">Card features</h2>
        <ul className="mt-3 divide-y divide-line border-y border-line">
          {feats.map((f) => (
            <li key={f.key} className="py-3 text-[14px]">
              <details>
                <summary className="flex cursor-pointer list-none flex-wrap items-center gap-2 [&::-webkit-details-marker]:hidden">
                  <span className={`font-semibold ${f.active ? "" : "text-muted line-through"}`}>{f.label}</span>
                  <span className="text-[12px] text-muted">{f.key} · {f.listings} listings</span>
                  {edit && <span className="ml-auto text-[13px] text-link underline">Edit</span>}
                </summary>
                {edit && (
                  <div className="mt-3 grid gap-4 md:grid-cols-[minmax(0,1fr)_auto]">
                    <ActionForm
                      endpoint={`/v1/admin/catalog/features/${f.key}`}
                      method="PUT"
                      fields={[
                        { name: "label", label: "Label", initial: f.label },
                        { name: "sort", label: "Order", initial: String(f.sort) },
                        { name: "active", label: "Available", type: "checkbox", initial: f.active },
                      ]}
                      transform={(v) => ({ ...v, sort: Number(v.sort) || 0 })}
                      submit="Save"
                      compact
                    />
                    {f.listings === 0 && <div><DeleteButton endpoint={`/v1/admin/catalog/features/${f.key}`} /></div>}
                  </div>
                )}
              </details>
            </li>
          ))}
        </ul>
        {edit && <AddFeatureForm />}
      </section>
    </div>
  );
}
